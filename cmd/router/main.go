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

	"github.com/Alekra1/sharded_key_value_store/internal/router"
)

func main() {
	addr := flag.String("addr", "localhost:8000", "address to listen on")
	statePath := flag.String("state", "", "router state file (required)")
	flag.Parse()

	if err := serve(*addr, *statePath); err != nil {
		fmt.Fprintln(os.Stderr, "router:", err)
		os.Exit(1)
	}
}

func serve(addr, statePath string) error {
	if statePath == "" {
		return errors.New("-state is required")
	}
	p, urls, err := router.LoadState(statePath)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("no cluster state in %s; initialise the cluster first", statePath)
	}
	if err != nil {
		return err
	}
	rt, err := router.New(p, urls)
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("listening on %s, nodes %v", addr, p.Members())
	return http.Serve(ln, rt.Handler())
}
