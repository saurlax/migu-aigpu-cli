//go:build !windows

package auth

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/zalando/go-keyring"
	"golang.org/x/sys/unix"
)

func (s *LocalStore) Load() (Session, error) {
	b, err := keyring.Get("migu-aigpu-cli", s.Dir)
	if err != nil {
		return Session{}, errors.New("cannot read OS keyring; unlock your keychain/Secret Service and run migu auth capture")
	}
	return Decode([]byte(b))
}
func (s *LocalStore) Save(session Session) error {
	if err := session.Validate(); err != nil {
		return err
	}
	b, err := json.Marshal(session)
	if err != nil {
		return errors.New("cannot encode session")
	}
	if err = keyring.Set("migu-aigpu-cli", s.Dir, string(b)); err != nil {
		return errors.New("cannot save to OS keyring; no plaintext fallback")
	}
	return nil
}
func (s *LocalStore) Delete() error {
	err := keyring.Delete("migu-aigpu-cli", s.Dir)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
func ImportLegacy(*LocalStore) error    { return errors.New("legacy DPAPI import requires Windows") }
func tryLock(f *os.File) error          { return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) }
func unlock(f *os.File)                 { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }
func replaceFile(from, to string) error { return os.Rename(from, to) }
