# PostgreSQL Dialect Extensions

Package `postgres` provides PostgreSQL-specific extensions for the `def` code generation tool.

Import path: `github.com/x5iu/def/dialects/postgres`

## API

### Returning

`Returning(columns ...any) any`

Generates a PostgreSQL `RETURNING` clause for `INSERT`, `UPDATE`, or `DELETE` operations.

- Without arguments: generates `RETURNING *` (all columns)
- With arguments: generates `RETURNING col1, col2, ...`

```go
import (
    "github.com/x5iu/def"
    "github.com/x5iu/def/dialects/postgres"
)

// RETURNING * (returns full row)
func (r *Repo) CreateUser(ctx context.Context, user *User) (*User, error) {
    return def.Create(user, postgres.Returning())
}

// RETURNING id (returns single column)
func (r *Repo) CreateUserReturnID(ctx context.Context, name string) (int64, error) {
    return def.Create(def.Set(user.Name, name), postgres.Returning(user.ID))
}

// RETURNING id, created_at (returns selected columns)
func (r *Repo) CreateUserPartial(ctx context.Context, name string) (*User, error) {
    return def.Create(
        def.Set(user.Name, name),
        postgres.Returning(user.ID, user.CreatedAt),
    )
}
```

### OnConflict

`OnConflict(columns ...any) ConflictTarget`

Specifies conflict target columns for a PostgreSQL `ON CONFLICT` clause (upsert). Returns a `ConflictTarget` which provides two resolution strategies:

#### ConflictTarget.DoNothing

`(ConflictTarget) DoNothing() any`

Generates `ON CONFLICT (...) DO NOTHING`. The insert is silently skipped when a conflict occurs on the target columns.

```go
// INSERT INTO role_permissions (role_id, resource, action) VALUES (...)
// ON CONFLICT (role_id, resource, action) DO NOTHING
func (r *Repo) UpsertPermission(ctx context.Context, roleID int64, resource, action string) (sql.Result, error) {
    var perm RolePermission
    return def.Create(
        def.Set(perm.RoleID, roleID),
        def.Set(perm.Resource, resource),
        def.Set(perm.Action, action),
        postgres.OnConflict(perm.RoleID, perm.Resource, perm.Action).DoNothing(),
    )
}
```

#### ConflictTarget.DoUpdate

`(ConflictTarget) DoUpdate(sets ...any) any`

Generates `ON CONFLICT (...) DO UPDATE SET ...`. Arguments must be `def.Set()` expressions specifying columns to update when a conflict occurs.

Use `def.Func[any]("EXCLUDED.column_name")` to reference the would-be inserted value in the `SET` clause.

```go
// INSERT INTO roles (name, created_at) VALUES (...)
// ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
// RETURNING *
func (r *Repo) UpsertRole(ctx context.Context, name string) (role *Role, err error) {
    return def.Create(
        def.Set(role.Name, name),
        def.Set(role.CreatedAt, def.Func[any]("now")),
        postgres.OnConflict(role.Name).DoUpdate(
            def.Set(role.Name, def.Func[any]("EXCLUDED.name")),
        ),
        postgres.Returning(),
    )
}
```

### Combining OnConflict with Returning

`OnConflict` and `Returning` can be used together. The generated SQL preserves the correct clause order: `INSERT ... VALUES (...) ON CONFLICT (...) DO ... RETURNING ...`

```go
// INSERT INTO settings (key, value) VALUES (...)
// ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
// RETURNING *
func (r *Repo) UpsertSetting(ctx context.Context, key, value string) (s *Setting, err error) {
    return def.Create(
        def.Set(s.Key, key),
        def.Set(s.Value, value),
        postgres.OnConflict(s.Key).DoUpdate(
            def.Set(s.Value, def.Func[any]("EXCLUDED.value")),
        ),
        postgres.Returning(),
    )
}
```
