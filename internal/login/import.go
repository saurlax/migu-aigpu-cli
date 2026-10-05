package login

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/saurlax/migu-aigpu-cli/internal/api"
	"github.com/saurlax/migu-aigpu-cli/internal/auth"
)

type validator func(context.Context, auth.Session) (auth.Session, error)

func importSession(ctx context.Context, reader Reader, store auth.Store, verify validator, interval time.Duration) (auth.Session, error) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		if ctx.Err() != nil {
			return auth.Session{}, errors.New("login cancelled or timed out; existing CLI session preserved")
		}
		data, err := reader.Read(ctx)
		if err != nil {
			return auth.Session{}, errors.New("login browser closed or disconnected; existing CLI session preserved")
		}
		if len(data) > 0 {
			state, err := auth.Decode(data)
			if err != nil {
				return auth.Session{}, errors.New("platform session fields changed; existing CLI session preserved")
			}
			state, err = verify(ctx, state)
			if err != nil {
				return auth.Session{}, errors.New("new login session failed platform verification; existing CLI session preserved")
			}
			// Do not hold the cross-process credential lock while the user types OTPs.
			release, err := store.Lock(ctx)
			if err != nil {
				return auth.Session{}, err
			}
			err = store.Save(state)
			release()
			if err != nil {
				return auth.Session{}, err
			}
			return state, nil
		}
		select {
		case <-ctx.Done():
			return auth.Session{}, errors.New("login cancelled or timed out; existing CLI session preserved")
		case <-tick.C:
		}
	}
}

type candidateStore struct {
	state auth.Session
	mu    sync.Mutex
}

func (s *candidateStore) Load() (auth.Session, error)          { return s.state, nil }
func (s *candidateStore) Save(state auth.Session) error        { s.state = state; return nil }
func (*candidateStore) Delete() error                          { return nil }
func (s *candidateStore) Lock(context.Context) (func(), error) { s.mu.Lock(); return s.mu.Unlock, nil }

func validate(ctx context.Context, state auth.Session, timeout time.Duration, team string) (auth.Session, error) {
	memory := &candidateStore{state: state}
	client := api.New(memory, timeout, team)
	if _, err := client.Call(ctx, "POST", api.Instance+"/search", map[string]any{"page": 1, "pageSize": 1}, true); err != nil {
		return auth.Session{}, err
	}
	return memory.state, nil
}
