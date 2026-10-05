// Package login implements foreground browser-assisted session import.
// It uses the platform's own login page, not an OAuth loopback callback.
package login

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
	"github.com/saurlax/migu-aigpu-cli/internal/api"
	"github.com/saurlax/migu-aigpu-cli/internal/auth"
)

// No form values or passwords are read. Return only the required session,
// after the exact platform origin and frontend configuration are present.
const extractSession = `() => {
 if (location.origin !== "https://aigpu.migu.cn") return null;
 const e = window.env;
 if (!e || !e.CLIENT_ID || !e.CLIENT_SECTET) return null;
 const access = localStorage.getItem("token"), refresh = localStorage.getItem("refresh_token");
 if (!access || !refresh) return null;
 const cookies = document.cookie.split(";").map(x => x.trim());
 const read = name => { const c = cookies.find(x => x.startsWith(name + "=")); return c ? decodeURIComponent(c.slice(c.indexOf("=") + 1)) : ""; };
 const t = Number(read("token_expires_time"));
 if (!Number.isFinite(t) || t <= 0) return null;
 return JSON.stringify({access_token:access, refresh_token:refresh, client_id:e.CLIENT_ID, client_secret:e.CLIENT_SECTET, refresh_at:t, expires_at:t+240, csrf:read("294f62ecd0")});
}`

type Reader interface {
	Read(context.Context) ([]byte, error)
	Close() error
}

type browserReader struct {
	browser  *rod.Browser
	launcher *launcher.Launcher
	profile  string
}

func findBrowser(spec string) (string, error) {
	if spec != "" {
		path, err := exec.LookPath(spec)
		if err != nil {
			return "", errors.New("browser executable not found; pass --browser with an installed Chrome/Edge path")
		}
		return path, nil
	}
	if path, ok := launcher.LookPath(); ok {
		return path, nil
	}
	return "", errors.New("Chrome/Edge not found; install a Chromium browser or use auth capture; no browser is downloaded automatically")
}

func browserLauncher(ctx context.Context, binary, profile string, headless bool) *launcher.Launcher {
	l := launcher.New().Context(ctx)
	// Use a minimal set rather than Rod's automation defaults, which disable
	// browser features such as phishing protection and site isolation.
	l.Flags = map[flags.Flag][]string{
		flags.Bin: {binary}, flags.UserDataDir: {profile}, flags.RemoteDebuggingPort: {"0"},
		"remote-debugging-address": {"127.0.0.1"}, "no-first-run": nil,
		"no-default-browser-check": nil,
		"disable-background-mode":  nil,
		flags.Arguments:            {"about:blank"},
	}
	// Current Windows Edge relaunches through a compatibility layer, detaching
	// the process and its stderr from the launcher. Keep our owned process attached.
	if runtime.GOOS == "windows" && strings.EqualFold(filepath.Base(binary), "msedge.exe") {
		l.Set("edge-skip-compat-layer-relaunch")
	}
	return l.HeadlessNew(headless).Leakless(false).Logger(io.Discard)
}

func openBrowser(ctx context.Context, binary string, headless bool, startURL string) (Reader, error) {
	profile, err := os.MkdirTemp("", "migu-login-")
	if err != nil {
		return nil, errors.New("cannot create temporary browser profile")
	}
	// Validate the generated absolute deletion target before any recursive cleanup.
	profile, err = filepath.Abs(profile)
	if err != nil || filepath.Dir(profile) != filepath.Clean(os.TempDir()) || !strings.HasPrefix(filepath.Base(profile), "migu-login-") {
		return nil, errors.New("unexpected browser profile path; cleanup aborted")
	}
	l := browserLauncher(ctx, binary, profile, headless)
	control, err := l.Launch()
	if err != nil {
		if l.PID() != 0 {
			l.Kill()
		}
		_ = os.RemoveAll(profile)
		return nil, errors.New("cannot launch browser; check its executable and desktop environment")
	}
	b := rod.New().NoDefaultDevice().ControlURL(control).Context(ctx)
	r := &browserReader{browser: b, launcher: l, profile: profile}
	if err = b.Connect(); err != nil {
		_ = r.Close()
		return nil, errors.New("cannot connect to the dedicated browser")
	}
	if _, err = b.Page(proto.TargetCreateTarget{URL: startURL}); err != nil {
		_ = r.Close()
		return nil, errors.New("cannot open the platform login page")
	}
	return r, nil
}

func (r *browserReader) Read(ctx context.Context) ([]byte, error) {
	probe, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pages, err := r.browser.Context(probe).Pages()
	if err != nil {
		if ctx.Err() == nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)) {
			return nil, nil
		}
		return nil, errors.New("dedicated browser disconnected or closed")
	}
	if len(pages) == 0 {
		return nil, errors.New("all login browser tabs were closed")
	}
	for _, page := range pages {
		value, err := page.Context(probe).Eval(extractSession)
		// Navigation destroys execution contexts. Treat it as not ready, and check
		// again while the command is active. Never print a raw CDP exception.
		if err != nil || value == nil || value.Type != proto.RuntimeRemoteObjectTypeString {
			continue
		}
		if text := value.Value.Str(); text != "" {
			return []byte(text), nil
		}
	}
	return nil, nil
}

func (r *browserReader) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := r.browser.Context(ctx).Close(); err != nil {
		r.launcher.Kill()
	}
	done := make(chan struct{})
	go func() { r.launcher.Cleanup(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		r.launcher.Kill()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			return errors.New("dedicated browser cleanup did not finish")
		}
	}
	// Cleanup's removal error is ignored upstream; verify removal explicitly.
	if _, err := os.Stat(r.profile); os.IsNotExist(err) {
		return nil
	}
	if err := os.RemoveAll(r.profile); err != nil {
		return errors.New("could not remove the temporary browser profile")
	}
	return nil
}

// Browser runs only for this command's lifetime. The old session is preserved
// unless a new candidate passes a real read-only platform query.
func Browser(ctx context.Context, store auth.Store, binary string, wait, timeout time.Duration, team string) (state auth.Session, err error) {
	if wait <= 0 || wait > 30*time.Minute {
		return state, errors.New("login wait must be between 0 and 30 minutes")
	}
	if timeout <= 0 {
		return state, errors.New("HTTP timeout must be positive")
	}
	binary, err = findBrowser(binary)
	if err != nil {
		return state, err
	}
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	reader, err := openBrowser(ctx, binary, false, api.Origin+"/train/home")
	if err != nil {
		return state, err
	}
	defer func() {
		if cleanupErr := reader.Close(); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
	}()
	return importSession(ctx, reader, store, func(ctx context.Context, s auth.Session) (auth.Session, error) {
		return validate(ctx, s, timeout, team)
	}, time.Second)
}
