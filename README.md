# Sharded key-value store

A sharded key-value store in Go built to compare consistent-hashing placement algorithms, all of them implementing one interface (`internal/placement`). Only Go standard library is used.

## Components

- `cmd/node`: storage server with in-memory map and a CRC32-checksum write-ahead log (`internal/store`).
- `cmd/router`: HTTP router that is responsible for cluster membership, placement state, request forwarding, and maintenance (`internal/router`).
- `cmd/kv`: CLI client for data and maintenance commands.
- `cmd/placesim`: in-process simulator that measures balance, add/remove disruption, saved state size, retained heap memory, and rebuild time of algorithms and different node counts.

Placement algorithms in `internal/placement`: `modulo`, `rendezvous`, `ring` (with virtual nodes), `jump`, and `maglev`.

## Build and test

```bash
go build -o ./ ./cmd/node ./cmd/router ./cmd/kv ./cmd/placesim
go test -race ./...
```

## Running a local cluster

Start three (or more) storage nodes (each in its own terminal, or in the background):

```bash
./node -init -addr localhost:8001 -data ./data/A
./node -init -addr localhost:8002 -data ./data/B
./node -init -addr localhost:8003 -data ./data/C
```

Create the cluster state file and start the router:

```bash
./router -init -addr localhost:8000 -state ./data/router.json \
  -placement ring -vnodes 100 \
  -nodes A=http://localhost:8001,B=http://localhost:8002,C=http://localhost:8003
```

On restart, start nodes and the router without `-init` using their existing data and state paths:

```bash
./node -addr localhost:8001 -data ./data/A
./router -addr localhost:8000 -state ./data/router.json
```

Read, write, delete, and see cluster state:

```bash
./kv put article:1 "first value"
./kv get article:1
./kv delete article:1
./kv status
```

## Maintenance operations

Start a fourth node, add it to the cluster, remove a node, and compact logs:

```bash
./node -init -addr localhost:8004 -data ./data/D
./kv node add D http://localhost:8004
./kv node remove B
./kv compact
```

During `node add`, `node remove`, and `compact`, the router pauses client requests, freezes participating nodes, moves and verifies records (for add/remove), updates `router.json`, cleans up old copies, unfreezes nodes, and resumes serving.

## Simulator and benchmarks

Run the placement simulator and write the results to the CSV file:

```bash
./placesim -keys 1000000 -seeds 3 -nodes 3,10,100,1000 -out results.csv
```

Measure the lookup time for each algorithm across parameters and node counts:

```bash
go test -bench=BenchmarkOwner -run=^$ ./internal/placement
```
