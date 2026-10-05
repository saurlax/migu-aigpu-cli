package login

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/saurlax/migu-aigpu-cli/internal/auth"
)

type fakeReader struct {
	data   []byte
	reads  int
	closed bool
	err    error
}

func (r *fakeReader) Read(context.Context) ([]byte, error) {
	r.reads++
	if r.reads == 1 && r.err == nil {
		return nil, nil
	}
	return r.data, r.err
}
func (r *fakeReader) Close() error { r.closed = true; return nil }

type fakeStore struct {
	candidateStore
	saves int
	locks int
}

func (s *fakeStore) Save(state auth.Session) error { s.saves++; return s.candidateStore.Save(state) }
func (s *fakeStore) Lock(ctx context.Context) (func(), error) {
	s.locks++
	return s.candidateStore.Lock(ctx)
}

func testSession() auth.Session {
	return auth.Session{Access: "fixture-access", Refresh: "fixture-refresh", ClientID: "fixture-id", ClientSecret: "fixture-secret", ExpiresAt: 2000000000, RefreshAt: 1999999760}
}

func TestVerifyBeforeSaveAndNavigationWait(t *testing.T) {
	input := testSession()
	data, _ := json.Marshal(input)
	reader := &fakeReader{data: data}
	store := &fakeStore{}
	verified := false
	got, err := importSession(context.Background(), reader, store, func(ctx context.Context, s auth.Session) (auth.Session, error) {
		if store.saves != 0 || store.locks != 0 {
			t.Fatal("credential lock/save before verification")
		}
		verified = true
		s.Access = "verified-fixture"
		return s, nil
	}, time.Millisecond)
	if err != nil || !verified || store.saves != 1 || got.Access != "verified-fixture" || reader.reads != 2 {
		t.Fatal("new session not verified and saved correctly")
	}
}

func TestFailureAndCancellationPreserveOldSession(t *testing.T) {
	valid, _ := json.Marshal(testSession())
	tests := []struct {
		name                 string
		data                 []byte
		readErr, errorVerify error
		cancel               bool
	}{
		{name: "verification", data: valid, errorVerify: errors.New("fixture-secret")},
		{name: "incomplete", data: []byte(`{"access_token":"fixture-access"}`)},
		{name: "browser closed", readErr: errors.New("fixture-secret")},
		{name: "cancelled", cancel: true},
		{name: "timeout"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			old := testSession()
			store := &fakeStore{candidateStore: candidateStore{state: old}}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			if test.cancel {
				cancel()
			}
			_, err := importSession(ctx, &fakeReader{data: test.data, err: test.readErr}, store, func(context.Context, auth.Session) (auth.Session, error) { return auth.Session{}, test.errorVerify }, time.Millisecond)
			if err == nil || store.saves != 0 || store.state != old || strings.Contains(err.Error(), "fixture-secret") {
				t.Fatal("failure changed state or leaked error")
			}
		})
	}
}
