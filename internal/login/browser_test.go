package login

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/saurlax/migu-aigpu-cli/internal/auth"
)

func TestLaunchFlagsAndExecutable(t *testing.T) {
	l := browserLauncher(context.Background(), "fixture-browser", "fixture-profile", false)
	if l.Get(flags.Bin) != "fixture-browser" || l.Get(flags.UserDataDir) != "fixture-profile" || l.Get(flags.RemoteDebuggingPort) != "0" || l.Get("remote-debugging-address") != "127.0.0.1" {
		t.Fatal("browser not isolated on loopback")
	}
	for _, unsafeFlag := range []flags.Flag{flags.Headless, flags.NoSandbox, "disable-web-security", "disable-client-side-phishing-detection", "disable-site-isolation-trials", "use-mock-keychain"} {
		if l.Has(unsafeFlag) {
			t.Fatalf("unexpected flag: %s", unsafeFlag)
		}
	}
	if _, err := findBrowser("nonexistent-migu-browser-fixture.exe"); err == nil {
		t.Fatal("missing executable accepted")
	}
}

// Opt-in smoke test uses an installed browser, synthetic credentials, and a
// localhost fixture. It does not access a live account or ask for an OTP.
func TestInstalledBrowserLifecycle(t *testing.T) {
	binary := os.Getenv("MIGU_BROWSER_TEST")
	if binary == "" {
		t.Skip("set MIGU_BROWSER_TEST to an installed Chrome/Edge executable")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>Browser login fixture</body></html>"))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	reader, err := openBrowser(ctx, binary, true, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	r := reader.(*browserReader)
	closed := false
	defer func() {
		if !closed {
			_ = reader.Close()
		}
	}()
	pages, err := r.browser.Pages()
	if err != nil || len(pages) == 0 {
		t.Fatal("no browser pages")
	}
	page := pages[0]
	_, err = page.Eval(`()=>{window.env={CLIENT_ID:"fixture-id",CLIENT_SECTET:"fixture-secret"};localStorage.setItem("token","fixture-access");localStorage.setItem("refresh_token","fixture-refresh");document.cookie="token_expires_time=1999999760; path=/";}`)
	if err != nil {
		t.Fatal(err)
	}
	data, err := reader.Read(ctx)
	if err != nil || len(data) != 0 {
		t.Fatal("non-platform origin session captured")
	}
	// Exercise the same extraction logic on the fixture by changing only its
	// allowed origin in this test, never in production.
	value, err := page.Eval(strings.Replace(extractSession, "https://aigpu.migu.cn", server.URL, 1))
	if err != nil {
		t.Fatal("session extraction failed")
	}
	state, err := auth.Decode([]byte(value.Value.Str()))
	if err != nil || state.Access != "fixture-access" || state.ClientSecret != "fixture-secret" || state.ExpiresAt != 2000000000 {
		t.Fatal("extracted wrong session fields")
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	closed = true
	if _, err = os.Stat(r.profile); !os.IsNotExist(err) {
		t.Fatal("temporary browser profile remains")
	}
}
