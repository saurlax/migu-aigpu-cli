//go:build windows

package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

func crypt(data []byte, decrypt bool) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty encrypted session")
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	var err error
	if decrypt {
		err = windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	if err != nil {
		return nil, errors.New("Windows DPAPI could not protect/unprotect session")
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(out.Data))))
	return append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...), nil
}

func (s *LocalStore) Load() (Session, error) {
	b, err := os.ReadFile(filepath.Join(s.Dir, "session.dpapi"))
	if err != nil {
		return Session{}, errors.New("no readable session; run migu auth capture")
	}
	b, err = crypt(b, true)
	if err != nil {
		return Session{}, err
	}
	return Decode(b)
}

func (s *LocalStore) Save(session Session) error {
	if err := session.Validate(); err != nil {
		return err
	}
	b, err := json.Marshal(session)
	if err != nil {
		return errors.New("cannot encode session")
	}
	b, err = crypt(b, false)
	if err != nil {
		return err
	}
	return atomicFile(filepath.Join(s.Dir, "session.dpapi"), b)
}

func (s *LocalStore) Delete() error {
	err := os.Remove(filepath.Join(s.Dir, "session.dpapi"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func ImportLegacy(s *LocalStore) error {
	b, err := os.ReadFile(filepath.Join(os.Getenv("LOCALAPPDATA"), "MiguEmotionCLI", "session.dpapi"))
	if err != nil {
		return errors.New("legacy Python session not found")
	}
	b, err = crypt(b, true)
	if err != nil {
		return err
	}
	session, err := Decode(b)
	if err != nil {
		return err
	}
	return s.Save(session)
}

func tryLock(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
}
func unlock(f *os.File) {
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &windows.Overlapped{})
}
func replaceFile(from, to string) error {
	a, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	b, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(a, b, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
