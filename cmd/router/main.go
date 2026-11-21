package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/Alekra1/sharded_key_value_store/internal/placement"
	"github.com/Alekra1/sharded_key_value_store/internal/router"
)

type config struct {
	addr, statePath string
	create          bool
	algorithm       string
	nodes           string
	vnodes, size    int
}

func main() {
	var c config
	flag.StringVar(&c.addr, "addr", "localhost:8000", "address to listen on")
	flag.StringVar(&c.statePath, "state", "", "router state file (required)")
	flag.BoolVar(&c.create, "init", false, "create a new cluster in the state file")
	flag.StringVar(&c.algorithm, "placement", "", "with -init: modulo, rendezvous, ring, jump or maglev")
	flag.StringVar(&c.nodes, "nodes", "", "with -init: A=http://localhost:8001,B=http://localhost:8002,...")
	flag.IntVar(&c.vnodes, "vnodes", 100, "with -init: virtual nodes per node for ring")
	flag.IntVar(&c.size, "size", 65537, "with -init: table size for maglev, must be prime")
	flag.Parse()

	if err := serve(c); err != nil {
		fmt.Fprintln(os.Stderr, "router:", err)
		os.Exit(1)
	}
}

func serve(c config) error {
	if c.statePath == "" {
		return errors.New("-state is required")
	}
	if c.create {
		if err := initCluster(c); err != nil {
			return err
		}
	} else if c.algorithm != "" || c.nodes != "" {
		return errors.New("-placement and -nodes are only used with -init")
	}

	rt, err := router.Open(c.statePath)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("no cluster state in %s; start with -init to create a cluster", c.statePath)
	}
	if err != nil {
		return err
	}
	if rt.Paused() {
		log.Printf("in maintenance; client requests are refused until the run is resumed")
	}

	ln, err := net.Listen("tcp", c.addr)
	if err != nil {
		return err
	}
	log.Printf("listening on %s, state in %s", c.addr, c.statePath)
	return http.Serve(ln, rt.Handler())
}

func initCluster(c config) error {
	if c.algorithm == "" || c.nodes == "" {
		return errors.New("-init needs -placement and -nodes")
	}
	urls, err := parseNodes(c.nodes)
	if err != nil {
		return err
	}
	err = router.InitState(c.statePath, c.algorithm, urls, c.vnodes, c.size)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%s already has a cluster; start without -init", c.statePath)
	}
	return err
}

func parseNodes(s string) (map[placement.NodeID]string, error) {
	urls := make(map[placement.NodeID]string)
	for _, part := range strings.Split(s, ",") {
		name, url, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || name == "" {
			return nil, fmt.Errorf("bad node %q, want NAME=URL", part)
		}
		if !strings.HasPrefix(url, "http://") {
			return nil, fmt.Errorf("node %s: URL %q must start with http://", name, url)
		}
		id := placement.NodeID(name)
		if _, dup := urls[id]; dup {
			return nil, fmt.Errorf("node %s given twice", name)
		}
		// A trailing slash would make the router ask for //kv, which the node answers
		// with a redirect.
		urls[id] = strings.TrimSuffix(url, "/")
	}
	return urls, nil
}
