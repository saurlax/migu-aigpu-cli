package auth

import (
	"context"
	"testing"
	"time"
)

func TestSessionValidationAndMargin(t *testing.T) {
	s := Session{Access: "a", Refresh: "r", ClientID: "c", ClientSecret: "s", ExpiresAt: float64(time.Now().Add(2 * time.Hour).Unix()), RefreshAt: float64(time.Now().Add(100 * time.Minute).Unix())}
	if s.Validate() != nil || s.Due(time.Now()) {
		t.Fatal("valid session rejected/due too early")
	}
	s.ExpiresAt = float64(time.Now().Add(59 * time.Minute).Unix())
	if !s.Due(time.Now()) {
		t.Fatal("one hour margin missing")
	}
	if _, err := Decode([]byte(`{"access_token":"a"}`)); err == nil {
		t.Fatal("incomplete session accepted")
	}
}

func TestCrossProcessLockPrimitive(t *testing.T) {
	s := &LocalStore{Dir: t.TempDir()}
	release, err := s.Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if second, err := s.Lock(ctx); err == nil {
		second()
		release()
		t.Fatal("second independent file lock acquired")
	}
	release()
	next, err := s.Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	next()
}
