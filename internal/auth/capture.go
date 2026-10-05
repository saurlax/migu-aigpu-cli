package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const origin = "https://aigpu.migu.cn"

// Capture is a foreground, one-shot loopback bridge. No credentials appear in
// the generated script or output; the logged-in page sends them directly.
func Capture(ctx context.Context, store *LocalStore, out io.Writer, duration time.Duration) error {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return errors.New("cannot create capture nonce")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return errors.New("cannot listen on loopback")
	}
	defer listener.Close()
	path := "/capture/" + hex.EncodeToString(nonce)
	endpoint := "http://" + listener.Addr().String() + path
	js := `(async()=>{if(location.origin!=="https://aigpu.migu.cn")throw Error("Wrong origin");const e=window.env||{};const cookies=document.cookie.split(';').map(x=>x.trim());const c=cookies.find(x=>x.startsWith('token_expires_time='));const csrf=cookies.find(x=>x.startsWith('294f62ecd0='));const t=Number(c?decodeURIComponent(c.slice(c.indexOf('=')+1)):0);const p={origin:location.origin,access_token:localStorage.getItem('token'),refresh_token:localStorage.getItem('refresh_token'),client_id:e.CLIENT_ID,client_secret:e.CLIENT_SECTET,refresh_at:t,expires_at:t?t+240:0,csrf:csrf?decodeURIComponent(csrf.slice(csrf.indexOf('=')+1)):null};const r=await fetch(%q,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(p)});if(!r.ok)throw Error('Import rejected');return 'Session imported locally';})()`
	// Separate captures use unique filenames so simultaneous attempts cannot
	// delete or overwrite another process's bridge.
	if err = os.MkdirAll(store.Dir, 0700); err != nil {
		return err
	}
	bridge := filepath.Join(store.Dir, "capture-"+hex.EncodeToString(nonce[:8])+".js")
	if err = os.WriteFile(bridge, []byte(fmt.Sprintf(js, endpoint)), 0600); err != nil {
		return err
	}
	defer os.Remove(bridge)
	fmt.Fprintln(out, "Log in at https://aigpu.migu.cn, then execute this local file's contents in that page's DevTools console:")
	fmt.Fprintln(out, bridge)
	fmt.Fprintln(out, "One-shot loopback capture; no credentials printed. Ctrl+C cancels.")
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	done := make(chan struct{}, 1)
	handler := newCaptureHandler(ctx, store, path, done)
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 20 * time.Second}
	failed := make(chan error, 1)
	go func() { failed <- server.Serve(listener) }()
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	select {
	case <-done:
		fmt.Fprintln(out, "Session imported securely.")
		return nil
	case <-ctx.Done():
		return errors.New("capture cancelled or timed out; no new session imported")
	case <-failed:
		return errors.New("capture listener stopped")
	}
}

func newCaptureHandler(ctx context.Context, store Store, path string, done chan<- struct{}) http.Handler {
	var mutex sync.Mutex
	imported := false
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != origin || r.URL.Path != path || r.URL.RawQuery != "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		mutex.Lock()
		defer mutex.Unlock()
		if imported {
			w.WriteHeader(http.StatusConflict)
			return
		}
		b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32768))
		var payload struct {
			Session
			Origin string `json:"origin"`
		}
		if err != nil || json.Unmarshal(b, &payload) != nil || payload.Origin != origin || payload.Validate() != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		release, err := store.Lock(ctx)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		err = store.Save(payload.Session)
		release()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		imported = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"imported":true}`)
		done <- struct{}{}
	})
}
