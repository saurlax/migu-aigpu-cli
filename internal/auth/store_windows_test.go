//go:build windows

package auth

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDPAPIRoundTripAndAtomicReplacement(t *testing.T) {
	s := &LocalStore{Dir: t.TempDir()}
	state := Session{Access: "synthetic-access", Refresh: "synthetic-refresh", ClientID: "id", ClientSecret: "synthetic-secret", ExpiresAt: 2000000000, RefreshAt: 1999999760}
	if err := s.Save(state); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(s.Dir, "session.dpapi"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte(state.ClientSecret)) {
		t.Fatal("plaintext secret on disk")
	}
	state.Access = "replacement"
	if err = s.Save(state); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil || got != state {
		t.Fatal("encrypted roundtrip/replacement failed")
	}
	if err = s.Save(Session{}); err == nil {
		t.Fatal("invalid overwrite accepted")
	}
	got, err = s.Load()
	if err != nil || got != state {
		t.Fatal("invalid save destroyed credentials")
	}
	if err = s.Delete(); err != nil {
		t.Fatal(err)
	}
}
