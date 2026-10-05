package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/saurlax/migu-aigpu-cli/internal/auth"
)

type memoryStore struct {
	s     auth.Session
	saves int
	mu    sync.Mutex
}

func (m *memoryStore) Load() (auth.Session, error)          { return m.s, nil }
func (m *memoryStore) Save(s auth.Session) error            { m.s = s; m.saves++; return nil }
func (*memoryStore) Delete() error                          { return nil }
func (m *memoryStore) Lock(context.Context) (func(), error) { m.mu.Lock(); return m.mu.Unlock, nil }
func session() auth.Session {
	return auth.Session{Access: "old-access", Refresh: "old-refresh", ClientID: "client", ClientSecret: "secret", ExpiresAt: float64(time.Now().Add(10 * time.Hour).Unix()), RefreshAt: float64(time.Now().Add(9 * time.Hour).Unix())}
}

func testClient(t *testing.T, store *memoryStore, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c := New(store, time.Second, "")
	c.base = server.URL
	return c
}

func TestReadRetryAndMutationNoReplay(t *testing.T) {
	for _, readOnly := range []bool{true, false} {
		t.Run(fmt.Sprint(readOnly), func(t *testing.T) {
			store := &memoryStore{s: session()}
			requests, refreshes := 0, 0
			c := testClient(t, store, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/v4/oauth/token" {
					refreshes++
					id, secret, ok := r.BasicAuth()
					if !ok || id != "client" || secret != "secret" {
						t.Error("missing Basic client auth")
					}
					if err := r.ParseForm(); err != nil {
						t.Error(err)
					}
					if r.Form.Get("refresh_token") != "old-refresh" {
						t.Error("wrong refresh token")
					}
					fmt.Fprint(w, `{"access_token":"new-access","refresh_token":"new-refresh","expires_in":43200}`)
					return
				}
				requests++
				if requests == 1 {
					w.WriteHeader(401)
					return
				}
				if r.Header.Get("Authorization") != "Bearer new-access" {
					t.Error("old access reused")
				}
				fmt.Fprint(w, `{"code":"1000","data":{"ok":true}}`)
			})
			_, err := c.Call(context.Background(), "POST", "/query", map[string]any{}, readOnly)
			if readOnly {
				if err != nil || requests != 2 || refreshes != 1 || store.s.Refresh != "new-refresh" {
					t.Fatalf("bad read retry: %v %d %d", err, requests, refreshes)
				}
			} else {
				if err == nil || requests != 1 || refreshes != 0 || store.saves != 0 {
					t.Fatal("mutation replayed")
				}
			}
		})
	}
}

func TestEarlyRenewAndConcurrentUse(t *testing.T) {
	store := &memoryStore{s: session()}
	store.s.ExpiresAt = float64(time.Now().Add(30 * time.Minute).Unix())
	c := testClient(t, store, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v4/oauth/token" {
			fmt.Fprint(w, `{"access_token":"new","refresh_token":"rotated","expires_in":43200}`)
		} else {
			fmt.Fprint(w, `{"code":1000,"data":{}}`)
		}
	})
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			if _, err := c.Call(context.Background(), "GET", "/query", nil, true); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if store.saves != 1 || store.s.Refresh != "rotated" || store.s.ExpiresAt-store.s.RefreshAt != 240 {
		t.Fatal("renewal not serialized/rotated")
	}
}

func TestFailedRenewPreservesSession(t *testing.T) {
	store := &memoryStore{s: session()}
	before := store.s
	c := testClient(t, store, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"error":"invalid_grant","secret":"must-not-print"}`)
	})
	_, _, err := c.Ensure(context.Background(), true)
	if err == nil || store.saves != 0 || store.s != before || strings.Contains(err.Error(), "must-not-print") {
		t.Fatal("failed renewal changed credentials or leaked error")
	}
}

func TestRedirectAndPathPinning(t *testing.T) {
	leaked := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer target.Close()
	c := testClient(t, &memoryStore{s: session()}, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) })
	if _, err := c.Call(context.Background(), "GET", "/redirect", nil, true); err == nil || leaked {
		t.Fatal("redirect followed")
	}
	for _, path := range []string{"//evil.example/path", "https://evil.example", "/../escape"} {
		if _, err := c.request(context.Background(), "GET", path, nil, nil, false); err == nil {
			t.Fatal("untrusted path accepted")
		}
	}
}

func TestRedaction(t *testing.T) {
	input := map[string]any{"password": "pw-fixture", "accessKey": "key-fixture", "environmentVarList": []any{map[string]any{"name": "NOTEBOOK_TOKEN", "value": "nb-fixture"}}, "url": "https://user:pass@example.com/?access_token=url-fixture&X-Amz-Signature=sig-fixture", "pem": "-----BEGIN RSA PRIVATE KEY-----fixture-----END RSA PRIVATE KEY-----", "refresh_token_present": true, "count": 12}
	b, _ := json.Marshal(Redact(input))
	s := string(b)
	for _, secret := range []string{"pw-fixture", "key-fixture", "nb-fixture", "url-fixture", "sig-fixture", "user:pass", "PRIVATE KEY"} {
		if strings.Contains(s, secret) {
			t.Errorf("leaked %s", secret)
		}
	}
	if !strings.Contains(s, `"count":12`) || !strings.Contains(s, `"refresh_token_present":true`) {
		t.Fatal("metadata lost")
	}
}
