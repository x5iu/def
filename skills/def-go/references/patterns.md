# def Patterns

## Table of Contents

1. [Preparation Checklist](#preparation-checklist)
2. [SELECT Patterns](#select-patterns)
3. [INSERT Patterns](#insert-patterns)
4. [UPDATE Patterns](#update-patterns)
5. [DELETE Patterns](#delete-patterns)
6. [PostgreSQL Patterns](#postgresql-patterns)
7. [SQL-to-DSL Mapping Checklist](#sql-to-dsl-mapping-checklist)

## Preparation Checklist

Use this checklist before writing or modifying methods:

- Ensure all referenced structs are bound in `def.Init(...)`.
- Ensure fields used in SQL have `db:"column_name"` tags.
- Ensure relation traversal fields have `foreign_key` tags.
- Ensure method params include all dynamic values used in `def.Filter`, `def.Set`, `def.Limit`, `def.Offset`.

## SELECT Patterns

### Select all with filter

```go
func (q *querier) GetUserByID(ctx context.Context, id int64) (user *User, err error) {
    def.Query(
        def.Filter(user.ID == id),
    )
    return nil, nil
}
```

### Select specific columns

```go
func (q *querier) GetUserBasic(ctx context.Context, id int64) (name string, age int, err error) {
    var user User
    def.Query(
        def.Column(user.Name),
        def.Column(user.Age),
        def.Filter(user.ID == id),
    )
    return "", 0, nil
}
```

### Use functions in filters

```go
func (q *querier) GetActiveCount(ctx context.Context) (count int64, err error) {
    var user User
    def.Query(
        def.Column(def.Count[int64](user.ID)),
        def.Filter(user.Status == "active"),
    )
    return 0, nil
}
```

### Use pagination

```go
func (q *querier) ListUsers(ctx context.Context, limit, offset int) (users []*User, err error) {
    def.Query(
        def.Limit(limit),
        def.Offset(offset),
    )
    return nil, nil
}
```

## INSERT Patterns

### Entity mode insert

```go
func (q *querier) CreateUser(ctx context.Context, user *User) (sql.Result, error) {
    return def.Create(user)
}
```

### Field mode insert

```go
func (q *querier) CreateUser(ctx context.Context, name string, age int) (sql.Result, error) {
    var user User
    return def.Create(
        def.Set(user.Name, name),
        def.Set(user.Age, age),
    )
}
```

## UPDATE Patterns

### Field mode update

```go
func (q *querier) UpdateUserName(ctx context.Context, id int64, name string) (sql.Result, error) {
    var user User
    return def.Update(
        def.Set(user.Name, name),
        def.Filter(user.ID == id),
    )
}
```

### Arithmetic/time expression update

```go
func (q *querier) BumpAttempt(ctx context.Context, id int64) (sql.Result, error) {
    var retry SettlementRetry
    return def.Update(
        def.Set(retry.Attempts, retry.Attempts+1),
        def.Set(retry.UpdatedAt, postgres.Now()),
        def.Filter(retry.ID == id),
    )
}
```

### CTE + UPDATE FROM + RETURNING

```go
func (q *querier) ClaimDueRetries(ctx context.Context, limit int) ([]*SettlementRetry, error) {
    var sr SettlementRetry
    var due SettlementRetry

    return def.Update(
        def.With(
            "due",
            def.From(sr),
            def.Column(sr.ID),
            def.Filter(sr.DoneAt == nil),
            def.Filter(sr.DeadAt == nil),
            def.Filter(sr.NextRetryAt <= postgres.Now()),
            def.OrderBy(
                def.Asc(sr.NextRetryAt),
                def.Asc(sr.ID),
            ),
            def.Limit(limit),
            postgres.ForUpdateSkipLocked(),
        ),
        def.Set(sr.Attempts, sr.Attempts+1),
        def.Set(sr.NextRetryAt, postgres.Now()+postgres.Interval("10 minutes")),
        def.Set(sr.UpdatedAt, postgres.Now()),
        def.From("due"),
        def.Filter(sr.ID == due.ID),
        postgres.Returning(sr.ID, sr.RequestID, sr.Payload, sr.Attempts),
    )
}
```

Expected SQL shape:

```sql
WITH due AS (
    SELECT id
    FROM settlement_retries
    WHERE done_at IS NULL
      AND dead_at IS NULL
      AND next_retry_at <= NOW()
    ORDER BY next_retry_at ASC, id ASC
    LIMIT ${limit}
    FOR UPDATE SKIP LOCKED
)
UPDATE settlement_retries
SET attempts = attempts + 1,
    next_retry_at = NOW() + INTERVAL '10 minutes',
    updated_at = NOW()
FROM due
WHERE settlement_retries.id = due.id
RETURNING id, request_id, payload, attempts
```

## DELETE Patterns

### Safe delete with filter

```go
func (q *querier) DeleteUser(ctx context.Context, id int64) (sql.Result, error) {
    var user User
    return def.Delete(
        def.Filter(user.ID == id),
    )
}
```

## PostgreSQL Patterns

### RETURNING struct rows

```go
func (q *querier) CreateAndGetUser(ctx context.Context, name string) (user *User, err error) {
    var userRow User
    return def.Create(
        def.Set(userRow.Name, name),
        postgres.Returning(),
    )
}
```

### UPSERT

```go
func (q *querier) UpsertRole(ctx context.Context, name string) (role *Role, err error) {
    var roleRow Role
    return def.Create(
        def.Set(roleRow.Name, name),
        postgres.OnConflict(roleRow.Name).DoUpdate(
            def.Set(roleRow.Name, postgres.Excluded(roleRow.Name)),
        ),
        postgres.Returning(),
    )
}
```

## SQL-to-DSL Mapping Checklist

Use this deterministic sequence when converting hand-written SQL:

1. Identify statement type: SELECT / INSERT / UPDATE / DELETE.
2. Identify target table type by `def.BindTable[T]("table")`.
3. Create or reuse a typed variable (`var user User`) for field paths.
4. Convert select list:
- `SELECT *` => no `def.Column`.
- explicit columns/functions => one `def.Column(...)` per item.
5. Convert predicates to `def.Filter(...)` with native Go operators.
6. Convert dynamic values to method parameters and reference by identifier.
7. Add pagination with `def.Limit` / `def.Offset` if SQL needs it.
8. Add PostgreSQL extras (`Returning`, `OnConflict`, `With`, `From`, `Now`, `Interval`) only when needed.
9. Run `def generate` and inspect generated SQL comment.
10. Run tests/lint and iterate until SQL shape matches intent.

