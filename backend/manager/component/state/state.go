package state

import (
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/pkg/errors"

	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

type State struct {
	TokenExpireCache *lru.Cache[string, bool]
	NonceManager     *NonceManager
	HeartbeatBuffer  *HeartbeatBuffer
}

// Get reports whether key is present in the token expire cache. It lets *State
// satisfy the auth package's TokenExpireCache interface directly, so callers
// can keep passing *State to auth.New while the interceptor itself only
// depends on this one lookup.
func (s *State) Get(key string) (bool, bool) {
	if s.TokenExpireCache == nil {
		return false, false
	}
	return s.TokenExpireCache.Get(key)
}

func New() (*State, error) {
	expireCache, err := lru.New[string, bool](128)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create auth expire cache")
	}
	return &State{
		TokenExpireCache: expireCache,
		NonceManager:     NewNonceManager(),
	}, nil
}

func NewWithStore(stores *store.Store) (*State, error) {
	s, err := New()
	if err != nil {
		return nil, err
	}
	s.HeartbeatBuffer = NewHeartbeatBuffer(stores, 0)
	return s, nil
}
