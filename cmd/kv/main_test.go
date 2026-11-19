package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Alekra1/sharded_key_value_store/internal/protocol"
)

// fakeRouter answers like the router in front of one node, with a map as the store.
func fakeRouter(t *testing.T) string {
	var mu sync.Mutex
	data := make(map[string]string)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /kv", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		value, ok := data[r.URL.Query().Get("key")]
		mu.Unlock()
		if !ok {
			reply(w, http.StatusNotFound, protocol.Error{Error: "key not found"})
			return
		}
		reply(w, http.StatusOK, protocol.Value{Value: &value})
	})
	mux.HandleFunc("PUT /kv", func(w http.ResponseWriter, r *http.Request) {
		var v protocol.Value
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil || v.Value == nil {
			reply(w, http.StatusBadRequest, protocol.Error{Error: "bad body"})
			return
		}
		mu.Lock()
		data[r.URL.Query().Get("key")] = *v.Value
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /kv", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		delete(data, r.URL.Query().Get("key"))
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// replyWith starts a router that gives the same reply to every request.
func replyWith(t *testing.T, status int, body string) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func reply(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

// kv runs one command and returns what it printed.
func kv(routerURL string, args ...string) (string, error) {
	var out bytes.Buffer
	err := run(routerURL, args, &out)
	return out.String(), err
}

func wantOutput(t *testing.T, routerURL string, want string, args ...string) {
	t.Helper()
	got, err := kv(routerURL, args...)
	if err != nil {
		t.Fatalf("kv %q: %v", args, err)
	}
	if got != want {
		t.Fatalf("kv %q printed %q, want %q", args, got, want)
	}
}

func wantError(t *testing.T, routerURL string, want string, args ...string) {
	t.Helper()
	_, err := kv(routerURL, args...)
	if err == nil || err.Error() != want {
		t.Fatalf("kv %q: error %v, want %q", args, err, want)
	}
}

func TestPutGetDelete(t *testing.T) {
	router := fakeRouter(t)

	wantOutput(t, router, "", "put", "cat", "meow")
	wantOutput(t, router, "meow\n", "get", "cat")
	wantOutput(t, router, "", "put", "cat", "purr")
	wantOutput(t, router, "purr\n", "get", "cat")
	wantOutput(t, router, "", "delete", "cat")
	wantError(t, router, `key "cat" not found`, "get", "cat")
	wantOutput(t, router, "", "delete", "cat")
}

func TestEmptyValue(t *testing.T) {
	router := fakeRouter(t)

	wantOutput(t, router, "", "put", "e", "")
	wantOutput(t, router, "\n", "get", "e")
}

func TestRequests(t *testing.T) {
	var mu sync.Mutex
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, r.Method+" "+r.URL.Path+
			" key="+r.URL.Query().Get("key")+
			" type="+r.Header.Get("Content-Type")+
			" body="+string(body))
		mu.Unlock()
		if r.Method == http.MethodGet {
			value := "v"
			reply(w, http.StatusOK, protocol.Value{Value: &value})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	// The slash at the end must not turn the path into //kv.
	router := srv.URL + "/"
	wantOutput(t, router, "", "put", "user 1+2", "a b")
	wantOutput(t, router, "v\n", "get", "user 1+2")
	wantOutput(t, router, "", "delete", "user 1+2")

	want := []string{
		`PUT /kv key=user 1+2 type=application/json body={"value":"a b"}`,
		"GET /kv key=user 1+2 type= body=",
		"DELETE /kv key=user 1+2 type= body=",
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(got, want) {
		t.Fatalf("router got\n%q\nwant\n%q", got, want)
	}
}

func TestRouterErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		args   []string
		want   string
	}{
		{"missing key", 404, `{"error":"key not found"}`, []string{"get", "x"}, `key "x" not found`},
		{"bad input", 400, `{"error":"key is empty"}`, []string{"put", "x", "v"}, "key is empty"},
		{"node down", 502, `{"error":"node B unavailable: refused"}`, []string{"put", "x", "v"}, "node B unavailable: refused"},
		{"node timeout", 504, `{"error":"node B did not answer in time; the outcome is unknown"}`, []string{"delete", "x"}, "node B did not answer in time; the outcome is unknown"},
		{"error without JSON", 500, "oops", []string{"delete", "x"}, "router replied 500 Internal Server Error"},
		{"value not JSON", 200, "meow", []string{"get", "x"}, `bad reply from router: "meow"`},
		{"value missing", 200, "{}", []string{"get", "x"}, `bad reply from router: "{}"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wantError(t, replyWith(t, c.status, c.body), c.want, c.args...)
		})
	}
}

func TestRouterDown(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	router := srv.URL
	srv.Close()

	_, err := kv(router, "get", "x")
	if err == nil || !strings.HasPrefix(err.Error(), "router unavailable: ") {
		t.Fatalf("error %v, want router unavailable", err)
	}
}

func TestBadCommands(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("bad command sent %s %s", r.Method, r.URL)
	}))
	defer srv.Close()

	for _, args := range [][]string{
		{},
		{"frob", "x"},
		{"put", "x"},
		{"put", "x", "v", "w"},
		{"get"},
		{"get", "a", "b"},
		{"delete"},
		{"delete", "a", "b"},
	} {
		if _, err := kv(srv.URL, args...); !errors.Is(err, errUsage) {
			t.Errorf("kv %q: error %v, want errUsage", args, err)
		}
	}
}
