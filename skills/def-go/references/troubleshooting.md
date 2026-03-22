# def Troubleshooting

Use this file when `def generate` fails or generated SQL does not match expected shape.

## Fast Debug Loop

1. Run generator: `go run ./cmd/def generate .`
2. Copy the exact error text.
3. Match error against the map below.
4. Apply fix.
5. Re-run generator and tests.

## Error Map

### `def.Filter requires exactly 1 argument`

Cause:
- Passed multiple expressions directly to one `def.Filter` call.

Fix:
- Keep one boolean expression inside each `def.Filter(...)`.
- Combine conditions with `&&`/`||` inside that expression, or use multiple `def.Filter(...)` calls.

### `def.Set requires exactly 2 arguments`

Cause:
- Missing field or value in `def.Set`.

Fix:
- Use `def.Set(fieldSelector, valueExpr)`.

### `def.Set first argument must be a field selector`

Cause:
- First argument is not `var.Field` shape.

Fix:
- Use a typed variable field, e.g. `def.Set(user.Name, name)`.

### `def.Update requires at least one Set expression`

Cause:
- Field-mode update has no `def.Set(...)`.

Fix:
- Add one or more `def.Set(...)` calls.

### `def.Update requires at least one Filter expression`

Cause:
- Update has no filter guard.

Fix:
- Add at least one `def.Filter(...)`.

### `def.Delete requires at least one Filter expression`

Cause:
- Delete has no filter guard.

Fix:
- Add at least one `def.Filter(...)`.

### `def.With("...") requires def.From(tableVar) to specify source table`

Cause:
- CTE missing source table binding.

Fix:
- Inside `def.With(...)`, add `def.From(typedVar)` where `typedVar` maps to a `def.BindTable` type.

### `def.From in def.Update requires a source name string literal`

Cause:
- Used variable/expression for outer update source.

Fix:
- Use string literal in update context, e.g. `def.From("due")`.

### `def.OrderBy requires at least 1 argument`

Cause:
- Empty `def.OrderBy()` call.

Fix:
- Add at least one item, optionally wrapped by `def.Asc(...)` / `def.Desc(...)`.

### `def.Asc/def.Desc requires exactly 1 argument`

Cause:
- Missing field or passed multiple fields.

Fix:
- Keep exactly one column expression per call.

### `postgres.Interval requires exactly 1 argument`

Cause:
- Interval called with 0 or multiple arguments.

Fix:
- Use one string literal, e.g. `postgres.Interval("10 minutes")`.

### `postgres.Interval argument must be a string literal`

Cause:
- Interval value passed as non-literal expression.

Fix:
- Pass a quoted string literal.

### `unknown identifier: xxx` (Limit/Offset parse)

Cause:
- `def.Limit(xxx)` or `def.Offset(xxx)` references a non-parameter identifier.

Fix:
- Use an integer literal or a method parameter name.

### `undefined: WithCache` / `undefined: Callback`

Cause:
- Business logic file references generated symbols before `def generate` has been run.
- Or the build tag setup is wrong — source and generated files are not properly separated.

Fix:
- Run `def generate --tags <tag> --defc-generate .` first to produce generated files.
- Ensure source file has `//go:build <tag>`, business logic file has `//go:build !<tag>`.
- Generated files automatically get `//go:build !<tag>`.

### `callback requires WithCache context`

Cause:
- Called `entity.Callback(ctx, store)` without initializing the cache context.

Fix:
- Call `ctx = WithCache(ctx)` before any `Callback` call.

### `def generate` fails with type errors from business logic file

Cause:
- Business logic file (which uses `WithCache`, `Callback`, `NewXxxStore`) is being loaded during `def generate`.

Fix:
- Add `//go:build !<tag>` to the business logic file so it is excluded when `def generate --tags <tag>` runs.

### Entity mode insert includes ID / auto-generated columns

Cause:
- `def.Create(entity)` inserts ALL fields with `db` tags, including `id`, unless the field is marked with `auto_increment:"true"`.

Fix:
- Add `auto_increment:"true"` tag to the field: `db:"id" primary_key:"true" auto_increment:"true"`.
- Or use field mode with explicit `def.Set(...)` to omit auto-generated columns.

## SQL Shape Mismatch Tips

- Query result order does not change after adding `def.OrderBy(...)`:
  - Cause: top-level `def.Query(...)` currently parses `Column/Filter/Limit/Offset` only.
  - Fix: do not rely on `def.OrderBy(...)` in `def.Query(...)`; if ordering is required for a mutation CTE flow, place it inside `def.With(...)`.

- Wrong table in SQL:
  - Confirm method return type / set field paths map to the intended bound struct type.
- Missing column qualification in `UPDATE ... FROM` join:
  - Ensure outer `def.From("cte_name")` exists and filter compares main vs source fields, e.g. `sr.ID == due.ID`.
- Missing `RETURNING`:
  - Add `postgres.Returning(...)` to mutation call.
- Missing `FOR UPDATE SKIP LOCKED` in CTE:
  - Add `postgres.ForUpdateSkipLocked()` inside `def.With(...)`.
