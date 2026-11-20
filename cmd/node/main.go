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

	"github.com/Alekra1/sharded_key_value_store/internal/store"
)

func main() {
	addr := flag.String("addr", "localhost:8001", "address to listen on")
	dir := flag.String("data", "", "data directory (required)")
	create := flag.Bool("init", false, "create a new empty store in the data directory")
	flag.Parse()

	if err := serve(*addr, *dir, *create); err != nil {
		fmt.Fprintln(os.Stderr, "node:", err)
		os.Exit(1)
	}
}

func serve(addr, dir string, create bool) error {
	if dir == "" {
		return errors.New("-data is required")
	}
	st, err := openStore(dir, create)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	// Every write is synced before it is acknowledged, so stopping the process
	// with Ctrl+C loses nothing and the store needs no shutdown step.
	log.Printf("listening on %s, data in %s", addr, dir)
	return http.Serve(ln, (&server{store: st}).routes())
}

func openStore(dir string, create bool) (*store.Store, error) {
	if create {
		st, err := store.Create(dir)
		if errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("%s already has a log; start without -init", dir)
		}
		return st, err
	}
	st, cut, err := store.Open(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("no log in %s; start with -init to create a new node", dir)
	}
	if err != nil {
		return nil, err
	}
	if _, frozen, _ := st.Status(); frozen {
		log.Printf("frozen for maintenance; writes are refused until the router unfreezes this node")
	}
	if cut > 0 {
		log.Printf("cut %d bytes from the end of the log; they are saved in a log.cut-* file in %s", cut, dir)
	}
	return st, nil
}
