package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type captureStore struct {
	saved   int
	session Session
}

func (s *captureStore) Load() (Session, error)             { return s.session, nil }
func (s *captureStore) Save(v Session) error               { s.saved++; s.session = v; return nil }
func (*captureStore) Delete() error                        { return nil }
func (*captureStore) Lock(context.Context) (func(), error) { return func() {}, nil }

func TestCaptureOriginNonceSizeAndOneShot(t *testing.T) {
	store := &captureStore{}
	done := make(chan struct{}, 1)
	handler := newCaptureHandler(context.Background(), store, "/capture/nonce", done)
	payload := fmt.Sprintf(`{"origin":%q,"access_token":"fixture-access","refresh_token":"fixture-refresh","client_id":"fixture-id","client_secret":"fixture-secret","expires_at":2000000000,"refresh_at":1999999760}`, origin)
	tests := []struct {
		method, path, from, body string
		want                     int
	}{
		{"POST", "/capture/nonce", "https://evil.example", payload, 403},
		{"POST", "/capture/wrong", origin, payload, 403},
		{"POST", "/capture/nonce?q=x", origin, payload, 403},
		{"OPTIONS", "/capture/nonce", origin, "", 204},
		{"GET", "/capture/nonce", origin, "", 405},
		{"POST", "/capture/nonce", origin, strings.Repeat("x", 32769), 400},
		{"POST", "/capture/nonce", origin, `{"origin":"https://evil.example"}`, 400},
		{"POST", "/capture/nonce", origin, payload, 200},
		{"POST", "/capture/nonce", origin, payload, 409},
	}
	for _, test := range tests {
		r := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		r.Header.Set("Origin", test.from)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != test.want {
			t.Fatalf("%s %s: %d, want %d", test.method, test.path, w.Code, test.want)
		}
		if strings.Contains(w.Body.String(), "fixture-secret") {
			t.Fatal("credential echoed")
		}
		if w.Code == http.StatusNoContent && w.Header().Get("Access-Control-Allow-Origin") != origin {
			t.Fatal("incorrect CORS")
		}
	}
	if store.saved != 1 || len(done) != 1 || store.session.ClientSecret != "fixture-secret" {
		t.Fatal("capture not one-shot")
	}
}
