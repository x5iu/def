# Repository Guidelines

## Project Structure & Module Organization
- `cmd/def/`: CLI entrypoint (`def generate ...`).
- `internal/defgen/`: core generator pipeline (loading packages, parsing DSL calls, relation analysis, SQL rendering, and codegen).
- `dialects/postgres/`: PostgreSQL-specific DSL helpers (e.g., `Returning(...)`).
- `def.go`: public placeholder API used by source code scanned by the generator.
- Tests live next to code as `*_test.go` (notably `internal/defgen/defgen_test.go` for most behavior checks).

## Build, Test, and Development Commands
- `go build ./cmd/def`: build the CLI binary.
- `go run ./cmd/def generate .`: run generator for current package.
- `go test ./...`: run all unit tests.
- `go test -race ./...`: run tests with race detector (required for behavior changes).
- `go test -cover ./...`: quick coverage check across modules.
- `golangci-lint run ./...`: run lint checks used by this repo.

## Coding Style & Naming Conventions
- Follow idiomatic Go and always format with `gofmt`.
- Keep packages focused and small; prefer adding logic in `internal/defgen` helpers rather than large monolithic functions.
- Naming:
  - Exported identifiers: `PascalCase`.
  - Unexported helpers/tests: `camelCase`.
  - Tests: `TestXxx` with descriptive suffixes (e.g., `TestGenerateMutationSQL_UpdateWithoutFilterReturnsError`).
- Preserve deterministic output in codegen paths (sort map-derived collections before emitting code).

## Testing Guidelines
- Use Go’s standard `testing` package and table-driven tests for parser/SQL/codegen branches.
- For generator changes, test both success and fail-fast paths (invalid filters, missing bindings, ambiguous interfaces).
- Validate with at least:
  1. `go test -race ./...`
  2. `golangci-lint run ./...`

## Commit & Pull Request Guidelines
- Commit style in history is concise, imperative, and English-first: `Add ...`, `Fix ...`, `Enable ...`, `Change ...`, `Enforce ...`.
- Keep commit scope coherent (one logical change set per commit).
- PRs should include:
  - What changed and why.
  - Risk/compatibility notes (especially SQL generation or interface matching changes).
  - Verification commands run and results.
