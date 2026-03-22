---
name: def-go
description: Design, convert, and troubleshoot SQL definitions written with github.com/x5iu/def in Go projects. Use when translating handwritten SQL into def.Query/Create/Update/Delete, adding PostgreSQL clauses (Returning, OnConflict, Excluded, Now, Interval, ForUpdateSkipLocked), implementing WITH plus UPDATE FROM flows with def.With/def.From, setting up foreign_key relations and Callback loading, running def generation, or fixing parser and SQL-shape mismatches.
---

# def-go Skill

Produce valid `def` DSL that `def generate` can parse and emit as intended SQL comments.

## Workflow

1. Inspect project wiring before editing method bodies.
- Confirm `var _ = def.Init(def.BindTable[T]("table"), ...)` includes every table type you will reference.
- Confirm struct fields used in SQL have `db:"column_name"` tags.
- Confirm primary key fields have `primary_key:"true"` tags.
- Confirm auto-increment primary key fields also have `auto_increment:"true"` tags (excluded from entity-mode INSERT).
- Confirm relation fields have `db:"-" foreign_key:"column_name"` tags (and optionally `inverse:"FieldName"` for has-many).
- Confirm imports:
  - `github.com/x5iu/def`
  - `github.com/x5iu/def/dialects/postgres` when using PostgreSQL helpers.

2. Set up the project file structure.
- Source file with `//go:build <tag>`: contains `def.Init`, interface declaration, querier struct, DSL methods.
- Business logic file with `//go:build !<tag>`: contains code that uses generated symbols (`WithCache`, `Callback`, `NewXxxStore`).
- Generated files (`//go:build !<tag>`): produced by `def generate`, should not be manually edited.
- Add `//go:generate def generate --tags <tag> --defc-generate -o <output>.go .` to the source file.

3. Select DSL shape by SQL statement type.
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

4. Write DSL methods correctly.
- DSL calls (`def.Query`, `def.Create`, `def.Update`, `def.Delete`) are **statements**, not return values.
- All methods must `return nil, nil` (or zero values). `def generate` replaces the body.

5. Translate predicates and expressions into native Go syntax.
- Prefer native operators inside `def.Filter`: `==`, `!=`, `>`, `<`, `>=`, `<=`, `&&`, `||`.
- Use `def.In(field, values)` for SQL `IN`.
- Use `field == nil` / `field != nil`, or `def.IsNull(field)` / `def.IsNotNull(field)` for NULL checks.
- Use `def.Func[T]("FUNC_NAME", args...)` for custom SQL functions.
- Keep names aligned with method parameters so placeholders render as `${param}`.

6. Add PostgreSQL helpers when needed.
- `postgres.Returning(...)` => `RETURNING`.
- `postgres.OnConflict(...).DoNothing()` / `.DoUpdate(def.Set(...))` => UPSERT behavior.
- `postgres.Excluded(field)` for `EXCLUDED.column`.
- `postgres.Now()` and `postgres.Interval("10 minutes")` for temporal expressions.
- `postgres.ForUpdateSkipLocked()` only inside `def.With(...)`.

7. Implement `WITH ... UPDATE ... FROM ...` with no alias-specific API.
- Build CTE with `def.With("cte_name", ...)`.
- Inside `def.With`, pass table variable to `def.From(tableVar)`.
- Add optional clauses: `def.Column`, `def.Filter`, `def.OrderBy`, `def.Limit`, `def.Offset`, `postgres.ForUpdateSkipLocked()`.
- In outer update, add `def.From("cte_name")`.
- Use native field comparison for join condition, e.g. `def.Filter(sr.ID == due.ID)`.

8. Use foreign_key relations and Callback for loading related data.
- Add `foreign_key:"column_name"` on the child struct's relation field (belongs-to direction).
- Add `inverse:"FieldName"` to tell `def` which field on the parent struct holds the has-many slice.
- `def generate` auto-produces: relation query methods, `Callback` methods on entities, `WithCache` utility.
- Relations are **not** auto-loaded. Call `Callback(ctx, store)` explicitly with a `WithCache(ctx)` context.

9. Generate and verify.
- Run generator: `def generate --tags <tag> --defc-generate -o <output>.go .`
- Or via go:generate: `go generate --tags <tag> ./...`
- Run checks:
  - `go test ./...`
  - `go test -race ./...`
  - `golangci-lint run ./...` when available
- Confirm generated SQL comments match intended shape.

## Struct Tag Reference

| Tag | Where | Purpose |
|-----|-------|---------|
| `db:"column_name"` | DB column fields | Maps field to SQL column |
| `db:"-"` | Relation / ignored fields | Excludes field from SQL columns |
| `primary_key:"true"` | ID fields | Marks primary key for relation queries |
| `auto_increment:"true"` | Auto-increment ID fields | Excludes field from entity-mode INSERT |
| `foreign_key:"column_name"` | Relation fields (`db:"-"`) | Specifies which column is the FK; defines belongs-to |
| `inverse:"FieldName"` | Relation fields with `foreign_key` | Names the has-many field on the referenced struct |

## `def generate` CLI Flags

| Flag | Description |
|------|-------------|
| `--tags <tags>` | Build tags for parsing source files (e.g., `sqlite`, `postgres`) |
| `-o <file>` | Output file path (default: `def_gen.go`) |
| `-T <name>` | Interface name (auto-detected if omitted) |
| `--defc-generate` | Directly invoke defc after intermediate generation (one-step) |
| `--defc <cmd>` | Custom defc command for `//go:generate` directive |
| `--defc-features <f>` | Additional defc features (e.g., `sqlx/rebind,sqlx/in`) |
| `--tx` | Always generate `WithTx` method |
| `--tx-type <type>` | Customize `WithTx` fn argument type |
| `--tx-isolation <level>` | Set isolation level for `WithTx` |

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

## Current Behavior Notes

- `def.Query(...)` parses `def.Column`, `def.Filter`, `def.Limit`, and `def.Offset`.
- `def.OrderBy(...)` at top-level `def.Query` is not parsed and is effectively ignored.

## References

- Use [patterns](references/patterns.md) for templates and SQL-to-DSL mappings.
- Use [troubleshooting](references/troubleshooting.md) for error-to-fix lookup.
