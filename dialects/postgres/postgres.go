package postgres

// Returning specifies columns for PostgreSQL RETURNING clause.
// Without arguments, generates RETURNING * when return type is a struct.
// With arguments, generates RETURNING col1, col2, ...
//
// Example usage:
//
//	// RETURNING * (auto-inferred from return type)
//	func (r *UserRepo) CreateUser(ctx context.Context, user *User) (*User, error) {
//	    return def.Create(user, postgres.Returning())
//	}
//
//	// RETURNING id
//	func (r *UserRepo) CreateUserReturnID(ctx context.Context, name string) (int64, error) {
//	    return def.Create(def.Set(user.Name, name), postgres.Returning(user.ID))
//	}
//
//	// RETURNING id, created_at
//	func (r *UserRepo) CreateUserPartial(ctx context.Context, name string) (*User, error) {
//	    return def.Create(def.Set(user.Name, name), postgres.Returning(user.ID, user.CreatedAt))
//	}
func Returning(columns ...any) any { return nil }

// ConflictTarget represents an ON CONFLICT target for PostgreSQL upsert operations.
// It is returned by [OnConflict] and provides [ConflictTarget.DoNothing] and
// [ConflictTarget.DoUpdate] methods to specify the conflict resolution action.
type ConflictTarget struct{}

// OnConflict specifies conflict target columns for a PostgreSQL ON CONFLICT clause.
// Use with [ConflictTarget.DoNothing] or [ConflictTarget.DoUpdate] to define the
// conflict resolution action.
//
// Example usage:
//
//	// ON CONFLICT (role_id, resource, action) DO NOTHING
//	func (r *Repo) UpsertPermission(ctx context.Context, roleID int64, resource, action string) (sql.Result, error) {
//	    return def.Create(
//	        def.Set(perm.RoleID, roleID),
//	        def.Set(perm.Resource, resource),
//	        def.Set(perm.Action, action),
//	        postgres.OnConflict(perm.RoleID, perm.Resource, perm.Action).DoNothing(),
//	    )
//	}
//
//	// ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
//	func (r *Repo) UpsertRole(ctx context.Context, name string) (role *Role, err error) {
//	    return def.Create(
//	        def.Set(role.Name, name),
//	        def.Set(role.CreatedAt, def.Func[any]("now")),
//	        postgres.OnConflict(role.Name).DoUpdate(
//	            def.Set(role.Name, postgres.Excluded(role.Name)),
//	        ),
//	        postgres.Returning(),
//	    )
//	}
func OnConflict(columns ...any) ConflictTarget { return ConflictTarget{} }

// DoNothing generates ON CONFLICT (...) DO NOTHING.
// The insert is silently skipped when a conflict occurs on the target columns.
func (ConflictTarget) DoNothing() any { return nil }

// DoUpdate generates ON CONFLICT (...) DO UPDATE SET ...
// Arguments should be def.Set() expressions specifying columns to update when a
// conflict occurs. Use [Excluded] to reference the would-be inserted value.
func (ConflictTarget) DoUpdate(sets ...any) any { return nil }

// Excluded references a column from PostgreSQL's EXCLUDED pseudo-table in an
// ON CONFLICT DO UPDATE SET clause. The argument should be a struct field
// expression identifying the column. Preview breaking change:
// def.Func("EXCLUDED.column") is no longer supported.
//
// Example:
//
//	postgres.OnConflict(role.Name).DoUpdate(
//	    def.Set(role.Name, postgres.Excluded(role.Name)),
//	)
func Excluded(column any) any { return nil }
