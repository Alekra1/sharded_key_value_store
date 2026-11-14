package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const logName = "log"

var (
	ErrEmptyKey     = errors.New("key is empty")
	ErrKeyTooLong   = fmt.Errorf("key is longer than %d bytes", maxKeyLen)
	ErrValueTooLong = fmt.Errorf("value is longer than %d bytes", maxValueLen)
)

type Store struct {
	mu     sync.RWMutex
	data   map[string]string
	log    *os.File
	broken error
}

func Create(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, logName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	_, err = f.Write(encodeHeader())
	if err == nil {
		err = f.Sync()
	}
	if err != nil {
		f.Close()
		os.Remove(path)
		return nil, err
	}
	return &Store{data: make(map[string]string), log: f}, nil
}

func (s *Store) Put(key, value string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	if len(value) > maxValueLen {
		return ErrValueTooLong
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.broken != nil {
		return s.broken
	}
	if err := s.writeEntry(entry{op: opPut, key: key, value: value}); err != nil {
		return err
	}
	s.data[key] = value
	return nil
}

func (s *Store) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.data[key]
	return value, ok
}

func (s *Store) Delete(key string) error {
	if err := checkKey(key); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.broken != nil {
		return s.broken
	}
	if _, ok := s.data[key]; !ok {
		return nil
	}
	if err := s.writeEntry(entry{op: opDelete, key: key}); err != nil {
		return err
	}
	delete(s.data, key)
	return nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.log.Close()
}

// After a failed write the file may end inside an entry. Writing more would
// put that damage in the middle of the log, where replay can't cut it off.
func (s *Store) writeEntry(e entry) error {
	if _, err := s.log.Write(encodeEntry(e)); err != nil {
		s.broken = fmt.Errorf("store stopped after a failed log write: %w", err)
		return s.broken
	}
	if err := s.log.Sync(); err != nil {
		s.broken = fmt.Errorf("store stopped after a failed log sync: %w", err)
		return s.broken
	}
	return nil
}

func checkKey(key string) error {
	if key == "" {
		return ErrEmptyKey
	}
	if len(key) > maxKeyLen {
		return ErrKeyTooLong
	}
	return nil
}
