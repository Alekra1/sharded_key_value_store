package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/Alekra1/sharded_key_value_store/internal/protocol"
	"github.com/Alekra1/sharded_key_value_store/internal/store"
)

var (
	errKeyNotFound    = errors.New("key not found")
	errMissingValue   = errors.New(`body has no "value" field`)
	errMissingRecords = errors.New(`body has no "records" field`)
	errMissingKeys    = errors.New(`body has no "keys" field`)
)

type server struct {
	store *store.Store
}

func (s *server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /kv", s.get)
	mux.HandleFunc("PUT /kv", s.put)
	mux.HandleFunc("DELETE /kv", s.delete)
	mux.HandleFunc("GET /status", s.status)
	mux.HandleFunc("POST /freeze", s.freeze)
	mux.HandleFunc("POST /unfreeze", s.unfreeze)
	mux.HandleFunc("GET /records", s.records)
	mux.HandleFunc("PUT /records", s.putRecords)
	mux.HandleFunc("DELETE /records", s.deleteRecords)
	return mux
}

func (s *server) get(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		writeError(w, http.StatusBadRequest, store.ErrEmptyKey)
		return
	}
	value, ok := s.store.Get(key)
	if !ok {
		writeError(w, http.StatusNotFound, errKeyNotFound)
		return
	}
	writeJSON(w, http.StatusOK, protocol.Value{Value: &value})
}

func (s *server) put(w http.ResponseWriter, r *http.Request) {
	var body protocol.Value
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("body is not valid JSON: %w", err))
		return
	}
	if body.Value == nil {
		writeError(w, http.StatusBadRequest, errMissingValue)
		return
	}
	key := r.URL.Query().Get("key")
	if err := s.store.Put(key, *body.Value); err != nil {
		writeError(w, storeErrorStatus(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) delete(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if err := s.store.Delete(key); err != nil {
		writeError(w, storeErrorStatus(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) status(w http.ResponseWriter, r *http.Request) {
	keys, frozen, broken := s.store.Status()
	st := protocol.NodeStatus{Keys: keys, Frozen: frozen}
	if broken != nil {
		st.Broken = broken.Error()
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *server) freeze(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Freeze(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) unfreeze(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Unfreeze(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) records(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, protocol.Records{Records: s.store.Records()})
}

func (s *server) putRecords(w http.ResponseWriter, r *http.Request) {
	var body protocol.Records
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("body is not valid JSON: %w", err))
		return
	}
	if body.Records == nil {
		writeError(w, http.StatusBadRequest, errMissingRecords)
		return
	}
	if err := s.store.PutAll(body.Records); err != nil {
		writeError(w, storeErrorStatus(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) deleteRecords(w http.ResponseWriter, r *http.Request) {
	var body protocol.Keys
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("body is not valid JSON: %w", err))
		return
	}
	if body.Keys == nil {
		writeError(w, http.StatusBadRequest, errMissingKeys)
		return
	}
	if err := s.store.DeleteAll(body.Keys); err != nil {
		writeError(w, storeErrorStatus(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func storeErrorStatus(err error) int {
	if errors.Is(err, store.ErrFrozen) {
		return http.StatusServiceUnavailable
	}
	if errors.Is(err, store.ErrEmptyKey) || errors.Is(err, store.ErrKeyTooLong) || errors.Is(err, store.ErrValueTooLong) {
		return http.StatusBadRequest
	}
	// Any other error comes from a failed log write, so the store is broken.
	return http.StatusInternalServerError
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, protocol.Error{Error: err.Error()})
}
