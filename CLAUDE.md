# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

```bash
# Build
go build ./cmd/def

# Lint (preferred over go vet / go build for verification)
golangci-lint run ./...

# Run all tests
go test ./...

# Check coverage (must be >95% for every package)
go test -cover ./...

# Run tests for the core package
go test ./internal/defgen -v

# Run a single test
go test ./internal/defgen -run TestFormatFilterTree -v

# Run a specific subtest
go test ./internal/defgen -run TestFormatFilterTree/simple_equal -v

# Run the generator
go run ./cmd/def generate .
go run ./cmd/def generate ./...
```

## Testing Requirements

- Run `go test -cover ./...` when validating changes.
- Test coverage for every package must be strictly greater than 95%.
- Do not consider a task complete if any package coverage is at or below 95%.

## Project Overview

`def` is a code generation tool that scans Go code containing `def.Query`, `def.Create`, `def.Update`, `def.Delete` definitions and generates Go interfaces with SQL comment annotations. The generated interfaces are consumed by a downstream tool (`defc`) which generates the actual implementations.

## Architecture: Processing Pipeline

The core logic lives in `internal/defgen/` and follows a multi-stage pipeline:

```
Source Code → Load → Parse → Analyze → SQL Generate → Code Generate → .go file
```

### Stage 1: Package Loading (`parse.go` — `Load()`)
Uses `golang.org/x/tools/go/packages` to load Go packages with full type information. Supports build tags via `--tags` flag.

### Stage 2: Parsing (`parse.go` + `schema.go` — `Parse()`)
Five sequential steps:
1. `parseAllStructs()` — Scans struct types for `db`, `primary_key`, `foreign_key` tags (`schema.go`)
2. `parseInterfaceDefs()` — Collects interface method signatures
3. `parseTableBindings()` — Finds `def.Init(def.BindTable[T]("table"))` calls
4. `parseQueryMethods()` — Finds `def.Query()` calls, extracts `Column/Filter/Limit/Offset`
5. `parseMutationMethods()` — Finds `def.Create/Update/Delete()` calls, extracts `Set/Filter`

Expression parsing builds tree structures: `parseFilterExprRecursive()` produces `FilterExpr` trees (And/Or/Comparison/In nodes), `parseFieldPath()` detects foreign key traversals.

### Stage 3: Analysis (`analyze.go`)
`AnalyzeFilter()` transforms parsed `FilterExpr` trees into `AnalyzedFilter` trees ready for SQL generation. Detects foreign key traversals and marks them for subquery generation.

### Stage 4: SQL Generation (`sql.go`)
- `GenerateSQL()` — Builds SELECT statements from `QueryMethod`
- `GenerateMutationSQL()` — Builds INSERT/UPDATE/DELETE from `MutationMethod`
- `formatFilterTree()` — Recursively formats filter trees into SQL WHERE clauses

### Stage 5: Relation Analysis (`relation.go`)
`AnalyzeRelations()` infers belongs-to and has-many relationships from foreign keys. Generates private relation methods and `Callback` methods for automatic relation loading with caching to prevent circular references.

### Stage 6: Code Generation (`codegen.go`)
`generateCode()` assembles the final `.go` file: package declaration, imports, `//go:generate` directive for `defc`, interface with SQL comments, and Callback methods. Output is formatted with `go/format` and `goimports`.

## Key Data Structures (`types.go`)

- `Package` — Top-level container: tables, methods, mutation methods, interfaces, relations
- `QueryMethod` — Parsed query: columns, filters, limit, offset, return type
- `MutationMethod` — Parsed mutation: kind (Create/Update/Delete), sets, filters, entity
- `FilterExpr` — Tree node: `FilterAnd`, `FilterOr`, `FilterComparison`, `FilterIn`
- `AnalyzedFilter` — SQL-ready filter with subquery detection for foreign keys
- `TableBinding` — Maps Go struct type to database table name with field/foreign key info
- `GenerateOptions` — CLI options: Output, Tags, InterfaceName, DefcCmd, DefcFeatures, DefcGenerate

## CLI (`cmd/def/main.go`)

Uses Cobra framework. Main command: `generate` (alias: `gen`).

Flags: `-o` (output file), `--tags` (build tags), `-T/--interface` (interface name), `--defc` (custom defc command for `//go:generate` directive — defaults to `go run -mod=mod github.com/x5iu/defc@latest`), `--defc-features` (additional defc features to include, e.g. `sqlx/rebind,sqlx/in`), `--defc-generate` (directly invoke defc instead of emitting a `//go:generate` directive).

## Dialect Support

`dialects/postgres/` provides PostgreSQL-specific features like `postgres.Returning()` for the RETURNING clause in mutation methods. When RETURNING is used, the method comment changes from `exec constbind` to `query constbind` and the return type changes from `sql.Result` to the specified type.

## Struct Tags

| Tag | Purpose | Example |
|-----|---------|---------|
| `db` | Column mapping (`-` for non-DB fields) | `db:"user_id"` |
| `primary_key` | Required for subquery/Callback generation | `primary_key:"true"` |
| `foreign_key` | Defines relationship, references local FK column | `foreign_key:"user_id"` |
| `inverse` | Custom has-many field name on referenced table (used with `foreign_key`) | `inverse:"Endpoints"` |

## Notable API Functions

- `def.IsNull(field)` / `def.IsNotNull(field)` — Generate `IS NULL` / `IS NOT NULL` conditions in filters. Works with `sql.Null*` types.
- `def.Count`, `def.Sum`, `def.Avg`, `def.Max`, `def.Min` — Aggregate functions usable in `def.Column` and `def.Filter`. Use generic type parameter (e.g. `def.Count[int64]`) when used in filter comparisons.
- `def.Func("NAME", args...)` — Custom SQL function calls. Use generic type parameter for filter comparisons. Arguments can be field references, string/number literals, or method parameters.
- `def.In(field, slice)` — Generates `IN (${slice})` clauses.
