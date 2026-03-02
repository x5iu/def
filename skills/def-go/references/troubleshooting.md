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

## SQL Shape Mismatch Tips

- Wrong table in SQL:
  - Confirm method return type / set field paths map to the intended bound struct type.
- Missing column qualification in `UPDATE ... FROM` join:
  - Ensure outer `def.From("cte_name")` exists and filter compares main vs source fields, e.g. `sr.ID == due.ID`.
- Missing `RETURNING`:
  - Add `postgres.Returning(...)` to mutation call.
- Missing `FOR UPDATE SKIP LOCKED` in CTE:
  - Add `postgres.ForUpdateSkipLocked()` inside `def.With(...)`.

