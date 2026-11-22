package store

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

const (
	logName    = "log"
	frozenName = "frozen"
)

var (
	ErrEmptyKey     = errors.New("key is empty")
	ErrKeyTooLong   = fmt.Errorf("key is longer than %d bytes", maxKeyLen)
	ErrValueTooLong = fmt.Errorf("value is longer than %d bytes", maxValueLen)
	ErrFrozen       = errors.New("node is frozen for maintenance")
)

type Store struct {
	mu     sync.RWMutex
	dir    string
	data   map[string]string
	log    *os.File
	frozen bool
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
	return &Store{dir: dir, data: make(map[string]string), log: f}, nil
}

// Open replays the log in dir and returns the store and the number of bytes
// cut from the end of the file. Cut bytes are saved to a log.cut-* file in dir.
func Open(dir string) (*Store, int, error) {
	path := filepath.Join(dir, logName)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	r := bytes.NewReader(raw)
	if err := checkHeader(r); err != nil {
		return nil, 0, err
	}

	data := make(map[string]string)
	goodEnd := len(raw) - r.Len()
	for {
		e, err := readEntry(r)
		// The first entry that can't be read ends the log, whether that is the
		// clean end of the file or an entry cut off by a crash.
		if err != nil {
			break
		}
		if e.op == opPut {
			data[e.key] = e.value
		} else {
			delete(data, e.key)
		}
		goodEnd = len(raw) - r.Len()
	}
	cut := len(raw) - goodEnd
	if cut > 0 {
		if err := saveCut(dir, raw[goodEnd:]); err != nil {
			return nil, 0, err
		}
	}

	frozen, err := fileExists(filepath.Join(dir, frozenName))
	if err != nil {
		return nil, 0, err
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return nil, 0, err
	}
	if cut > 0 {
		err = f.Truncate(int64(goodEnd))
		if err == nil {
			err = f.Sync()
		}
		if err != nil {
			f.Close()
			return nil, 0, err
		}
	}
	return &Store{dir: dir, data: data, log: f, frozen: frozen}, cut, nil
}

// Disk damage can sit in front of good entries, so the cut bytes are kept in a
// side file where a person can still recover them.
func saveCut(dir string, cutBytes []byte) error {
	f, err := os.CreateTemp(dir, logName+".cut-*")
	if err != nil {
		return err
	}
	_, err = f.Write(cutBytes)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
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
	if s.frozen {
		return ErrFrozen
	}
	if err := s.writeEntries(entry{op: opPut, key: key, value: value}); err != nil {
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
	if s.frozen {
		return ErrFrozen
	}
	if _, ok := s.data[key]; !ok {
		return nil
	}
	if err := s.writeEntries(entry{op: opDelete, key: key}); err != nil {
		return err
	}
	delete(s.data, key)
	return nil
}

// Status reports the number of keys, whether the store is frozen, and the
// error that broke the store, or nil.
func (s *Store) Status() (keys int, frozen bool, broken error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data), s.frozen, s.broken
}

// Freeze makes Put and Delete fail with ErrFrozen until Unfreeze. It takes the
// write lock, so a write in progress finishes first and none lands after
// Freeze returns. The frozen file keeps the store frozen after a restart.
func (s *Store) Freeze() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.frozen {
		return nil
	}
	if err := os.WriteFile(filepath.Join(s.dir, frozenName), nil, 0o644); err != nil {
		return err
	}
	s.frozen = true
	return nil
}

func (s *Store) Unfreeze() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(filepath.Join(s.dir, frozenName))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	s.frozen = false
	return nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.log.Close()
}

// After a failed write the file may end inside an entry. Writing more would
// put that damage in the middle of the log, where replay can't cut it off.
func (s *Store) writeEntries(entries ...entry) error {
	var buf []byte
	for _, e := range entries {
		buf = append(buf, encodeEntry(e)...)
	}
	if _, err := s.log.Write(buf); err != nil {
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
