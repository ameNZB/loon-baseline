package maintenance

import (
	"context"
	"sync"
)

// Mock is an in-memory Store for tests and no-database hosts.
type Mock struct {
	mu sync.Mutex
	s  State
}

func NewMock() *Mock { return &Mock{} }

var _ Store = (*Mock)(nil)

func (m *Mock) Get(context.Context) (State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.s, nil
}

func (m *Mock) Set(_ context.Context, s State) error {
	m.mu.Lock()
	m.s = s
	m.mu.Unlock()
	return nil
}
