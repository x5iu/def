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
