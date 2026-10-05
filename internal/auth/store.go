// Package auth stores only the session required to call the platform.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Session struct {
	Access       string  `json:"access_token"`
	Refresh      string  `json:"refresh_token"`
	ClientID     string  `json:"client_id"`
	ClientSecret string  `json:"client_secret"`
	RefreshAt    float64 `json:"refresh_at"`
	ExpiresAt    float64 `json:"expires_at"`
	LastRefresh  float64 `json:"last_refresh,omitempty"`
	CSRF         string  `json:"csrf,omitempty"`
}

func (s Session) Validate() error {
	if s.Access == "" || s.Refresh == "" || s.ClientID == "" || s.ClientSecret == "" || s.ExpiresAt <= 0 || s.ExpiresAt >= 1e11 || s.RefreshAt < 0 || s.RefreshAt >= 1e11 {
		return errors.New("incomplete session; import from the logged-in platform again")
	}
	return nil
}

func (s Session) Due(now time.Time) bool {
	t := float64(now.Unix())
	return t >= s.RefreshAt || t+3600 >= s.ExpiresAt
}

type Store interface {
	Load() (Session, error)
	Save(Session) error
	Delete() error
	Lock(context.Context) (func(), error)
}

type LocalStore struct{ Dir string }

func DefaultStore() (*LocalStore, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, errors.New("cannot locate user configuration directory")
	}
	return &LocalStore{Dir: filepath.Join(dir, "migu-aigpu-cli")}, nil
}

func (s *LocalStore) Lock(ctx context.Context) (func(), error) {
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return nil, errors.New("cannot create credential directory")
	}
	f, err := os.OpenFile(filepath.Join(s.Dir, "session.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.New("cannot open session lock")
	}
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if tryLock(f) == nil {
			return func() { unlock(f); _ = f.Close() }, nil
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-deadline.C:
			_ = f.Close()
			return nil, errors.New("another CLI process holds the session lock; retry later")
		case <-tick.C:
		}
	}
}

func Decode(data []byte) (Session, error) {
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return s, errors.New("invalid session data")
	}
	return s, s.Validate()
}

// AtomicFile writes only encrypted data on Windows. Unix uses the OS keyring.
func atomicFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return errors.New("cannot create credential directory")
	}
	f, err := os.CreateTemp(filepath.Dir(path), "session-*.tmp")
	if err != nil {
		return errors.New("cannot create credential file")
	}
	defer os.Remove(f.Name())
	_ = f.Chmod(0600)
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return errors.New("cannot write credential file")
	}
	if err = replaceFile(f.Name(), path); err != nil {
		return errors.New("cannot replace credential file")
	}
	return nil
}
