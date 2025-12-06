package metadata

import (
	"context"
	"sync"
)

// Store tracks metadata entries for stored files.
type Store interface {
	Init(context.Context) error
	Put(context.Context, Record) error
	Get(context.Context, string) (Record, bool, error)
	Close(context.Context) error
}

// Record captures basic metadata per file.
type Record struct {
	Key       string
	Checksum  string
	SizeBytes int64
	CreatedAt int64
	UpdatedAt int64
}

// Config allows configuring persistent storage.
type Config struct {
	Path string
}

// NewInMemoryStore provides a simple in-memory placeholder store.
func NewInMemoryStore(_ Config) Store {
	return &inMemoryStore{items: make(map[string]Record)}
}

type inMemoryStore struct {
	mu    sync.RWMutex
	items map[string]Record
}

func (s *inMemoryStore) Init(context.Context) error { return nil }

func (s *inMemoryStore) Put(_ context.Context, rec Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[rec.Key] = rec
	return nil
}

func (s *inMemoryStore) Get(_ context.Context, key string) (Record, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.items[key]
	return rec, ok, nil
}

func (s *inMemoryStore) Close(context.Context) error { return nil }
