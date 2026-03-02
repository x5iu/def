---
name: def-go
description: Design, convert, and troubleshoot SQL definitions built with github.com/x5iu/def in Go codebases. Use when translating SQL into def DSL (def.Query/Create/Update/Delete), adding PostgreSQL clauses (Returning, OnConflict, Excluded, Now, Interval, ForUpdateSkipLocked), implementing WITH + UPDATE ... FROM via def.With/def.From, running def generation, or fixing def parser/generator errors.
compatibility: Requires a Go project using github.com/x5iu/def and access to go commands (go run/go test). golangci-lint is optional.
metadata:
  author: def-maintainers
  version: "1.0.0"
---

# def-go Skill

Use this skill to produce valid `def` DSL that `def generate` can parse and emit as correct SQL comments.

## Activation Cues

Activate this skill when the task includes any of these:
- Convert handwritten SQL to `def` DSL.
- Add or edit `def.Query`, `def.Create`, `def.Update`, or `def.Delete`.
- Add PostgreSQL-specific clauses (`Returning`, `OnConflict`, `Excluded`, `Now`, `Interval`, `ForUpdateSkipLocked`).
- Implement queue-claim style SQL (`WITH ... FOR UPDATE SKIP LOCKED ... UPDATE ... FROM ... RETURNING`).
- Diagnose `def generate` parse/analysis errors.

## Workflow

1. Inspect project wiring before editing methods.
- Confirm `def.Init(def.BindTable[T]("table"), ...)` includes every table type you will reference.
- Confirm struct fields used in DSL have `db:"..."` tags.
- Confirm relation traversal fields used in filters have `foreign_key:"..."` tags.
- Confirm imports:
  - `github.com/x5iu/def`
  - `github.com/x5iu/def/dialects/postgres` when using PostgreSQL helpers.

2. Select DSL shape by SQL statement type.
- `SELECT` => `def.Query(...)`
  - Supported top-level args: `def.Column`, `def.Filter`, `def.Limit`, `def.Offset`.
  - Do not use top-level `def.OrderBy` in `def.Query` (currently supported in `def.With` flow).
- `INSERT` => `def.Create(...)`
  - Entity mode: `def.Create(entity)`
  - Field mode: `def.Create(def.Set(...), ...)`
- `UPDATE` => `def.Update(...)`
  - Entity mode: `def.Update(entity, def.Filter(...))`
  - Field mode: `def.Update(def.Set(...), ..., def.Filter(...))`
- `DELETE` => `def.Delete(...)`

3. Translate predicates into native Go expressions.
- Prefer native operators inside `def.Filter`: `==`, `!=`, `>`, `<`, `>=`, `<=`, `&&`, `||`.
- Use `def.In(field, values)` for SQL `IN`.
- Use `field == nil` / `field != nil`, or `def.IsNull(field)` / `def.IsNotNull(field)` for NULL checks.
- Use `def.Func[T]("FUNC_NAME", args...)` for custom SQL functions.
- Keep names aligned with method parameters so placeholders render as `${param}`.

4. Add PostgreSQL helpers when needed.
- `postgres.Returning(...)` => `RETURNING`.
- `postgres.OnConflict(...).DoNothing()` / `.DoUpdate(def.Set(...))` => UPSERT behavior.
- `postgres.Excluded(field)` for `EXCLUDED.column`.
- `postgres.Now()` and `postgres.Interval("10 minutes")` for temporal expressions.
- `postgres.ForUpdateSkipLocked()` only inside `def.With(...)`.

5. Implement `WITH ... UPDATE ... FROM ...` with no alias-specific API.
- Build CTE with `def.With("cte_name", ...)`.
- Inside `def.With`, pass table variable to `def.From(tableVar)`.
- Add optional clauses: `def.Column`, `def.Filter`, `def.OrderBy`, `def.Limit`, `def.Offset`, `postgres.ForUpdateSkipLocked()`.
- In outer update, add `def.From("cte_name")`.
- Use native field comparison for join condition, e.g. `def.Filter(sr.ID == due.ID)`.

6. Generate and verify.
- Run generator:
  - `go run ./cmd/def generate .`
  - or `def generate .`
- Run checks:
  - `go test ./...`
  - `go test -race ./...`
  - `golangci-lint run ./...` when available
- Confirm generated SQL comments match intended shape.

## Constraints And Guardrails

- `def.Filter(...)` accepts exactly one boolean expression.
- `def.Set(...)` accepts exactly two arguments: field selector and value expression.
- `def.Update(...)` and `def.Delete(...)` require at least one `def.Filter(...)`.
- `def.With(...)` requirements:
  - first argument is a non-empty string name
  - at least one inner clause
  - must include `def.From(tableVar)` to resolve source table
- `def.From(...)` context rules:
  - inside `def.With`: use table variable (`def.From(sr)`)
  - inside `def.Update`: use source string literal (`def.From("due")`)
- `def.OrderBy(...)` needs at least one item.
- `def.Asc(...)` and `def.Desc(...)` each take exactly one expression.
- `postgres.Interval(...)` takes exactly one string literal.

## References

- Use [patterns](references/patterns.md) for templates and SQL-to-DSL mappings.
- Use [troubleshooting](references/troubleshooting.md) for error-to-fix lookup.
