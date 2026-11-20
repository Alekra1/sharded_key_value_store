package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Alekra1/sharded_key_value_store/internal/protocol"
)

const usage = `usage:
  kv [-router URL] put KEY VALUE
  kv [-router URL] get KEY
  kv [-router URL] delete KEY
  kv [-router URL] status
`

var errUsage = errors.New("bad command")

func main() {
	routerURL := flag.String("router", "http://localhost:8000", "router address")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	err := run(*routerURL, flag.Args(), os.Stdout)
	if errors.Is(err, errUsage) {
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "kv:", err)
		os.Exit(1)
	}
}

func run(routerURL string, args []string, out io.Writer) error {
	c := client{
		base: strings.TrimSuffix(routerURL, "/"),
		// Longer than the router's 5 s wait for a node, so a slow node shows up
		// as the router's 504 and not as a timeout here.
		http: &http.Client{Timeout: 10 * time.Second},
	}
	if len(args) == 0 {
		return errUsage
	}
	cmd, rest := args[0], args[1:]

	switch {
	case cmd == "put" && len(rest) == 2:
		return c.put(rest[0], rest[1])
	case cmd == "get" && len(rest) == 1:
		value, err := c.get(rest[0])
		if err != nil {
			return err
		}
		fmt.Fprintln(out, value)
		return nil
	case cmd == "delete" && len(rest) == 1:
		return c.delete(rest[0])
	case cmd == "status" && len(rest) == 0:
		return c.status(out)
	}
	return errUsage
}

type client struct {
	base string
	http *http.Client
}

func (c client) put(key, value string) error {
	body, err := json.Marshal(protocol.Value{Value: &value})
	if err != nil {
		return err
	}
	_, err = c.do(http.MethodPut, kvPath(key), bytes.NewReader(body))
	return err
}

func (c client) get(key string) (string, error) {
	data, err := c.do(http.MethodGet, kvPath(key), nil)
	if errors.Is(err, errNotFound) {
		return "", fmt.Errorf("key %q not found", key)
	}
	if err != nil {
		return "", err
	}
	var v protocol.Value
	if err := json.Unmarshal(data, &v); err != nil || v.Value == nil {
		return "", fmt.Errorf("bad reply from router: %q", data)
	}
	return *v.Value, nil
}

func (c client) delete(key string) error {
	_, err := c.do(http.MethodDelete, kvPath(key), nil)
	return err
}

func (c client) status(out io.Writer) error {
	data, err := c.do(http.MethodGet, "/status", nil)
	if err != nil {
		return err
	}
	var st protocol.ClusterStatus
	if err := json.Unmarshal(data, &st); err != nil {
		return fmt.Errorf("bad reply from router: %q", data)
	}

	fmt.Fprintf(out, "placement: %s\n", st.Placement)
	keys, reached := 0, 0
	for _, n := range st.Nodes {
		if n.Unavailable != "" {
			fmt.Fprintf(out, "%s  %s  unavailable: %s\n", n.Name, n.URL, n.Unavailable)
			continue
		}
		line := fmt.Sprintf("%s  %s  %d keys", n.Name, n.URL, n.Keys)
		if n.Frozen {
			line += ", frozen"
		}
		if n.Broken != "" {
			line += ", broken: " + n.Broken
		}
		fmt.Fprintln(out, line)
		keys += n.Keys
		reached++
	}
	fmt.Fprintf(out, "total: %d keys on %d of %d nodes\n", keys, reached, len(st.Nodes))
	return nil
}

func kvPath(key string) string {
	return "/kv?key=" + url.QueryEscape(key)
}

var errNotFound = errors.New("not found")

func (c client) do(method, path string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, errors.New("router did not answer in time; the outcome is unknown")
		}
		return nil, fmt.Errorf("router unavailable: %v", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, errNotFound
	case resp.StatusCode >= 300:
		return nil, replyError(resp.Status, data)
	}
	return data, nil
}

// replyError prefers the router's own message, for example
// "node B unavailable: ...", over the bare status line.
func replyError(status string, data []byte) error {
	var e protocol.Error
	if json.Unmarshal(data, &e) != nil || e.Error == "" {
		return fmt.Errorf("router replied %s", status)
	}
	return errors.New(e.Error)
}
