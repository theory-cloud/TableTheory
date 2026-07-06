# Go Contract Runner (stub)

This runner executes the shared TableTheory contract scenarios from `contract-tests/scenarios/` against the Go TableTheory
implementation (this repo) using DynamoDB Local.

## Prereqs

- Docker running (for DynamoDB Local)

## Run

From the repo root:

```bash
docker compose -f contract-tests/docker-compose.yml up -d
cd contract-tests/runners/go
go test ./... -count=1 -v
```

## Notes

- This folder is a **nested Go module** so it won’t affect `go test ./...` from the parent repo.
- Use `-count=1` for local contract runs. The Go runner reads shared YAML fixtures outside the nested module, so fresh
  execution is the least-surprise policy for repeated harness runs.
- The `go.mod` uses a local `replace` to point at the parent `theorydb` module; remove it when extracting to a standalone
  `theorydb-contract-tests` repo.
