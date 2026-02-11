package def

// Init initializes table bindings for the code generator.
// It takes BindTable calls as arguments.
func Init(...any) any { return nil }

// Query defines a SQL query with filters and column selections.
// It takes Filter and Column calls as arguments.
func Query(...any) any { return nil }

// Filter adds WHERE conditions to a query.
// Supports boolean expressions with && (AND) and || (OR).
// Example: def.Filter(user.Status == "active" && user.ID == id)
func Filter(bool) any { return nil }

// BindTable binds a Go struct type to a database table name.
func BindTable[T any](string) any { return nil }

// In creates an IN query condition.
// Example: def.In(user.ID, ids) generates "id IN (${ids})"
func In(any, any) bool { return false }

// IsNull checks if a field is NULL.
// Works with any field type including sql.NullString, sql.NullInt64, sql.Null[T], etc.
// Example: def.Filter(def.IsNull(user.DeletedAt)) generates "WHERE deleted_at IS NULL"
func IsNull(any) bool { return false }

// IsNotNull checks if a field is not NULL.
// Works with any field type including sql.NullString, sql.NullInt64, sql.Null[T], etc.
// Example: def.Filter(def.IsNotNull(user.Name)) generates "WHERE name IS NOT NULL"
func IsNotNull(any) bool { return false }

// Column specifies a column to select in the query.
// Without Column calls, the query defaults to SELECT *.
// Example: def.Column(user.Name) generates "SELECT name"
func Column(any) any { return nil }

// Count creates a COUNT aggregate function.
// Example: def.Column(def.Count(user.ID)) generates "SELECT COUNT(id)"
// Example: def.Filter(def.Count[int64](t.ID) > 0) generates "WHERE COUNT(id) > 0"
func Count[T any](any) T { var zero T; return zero }

// Sum creates a SUM aggregate function.
// Example: def.Column(def.Sum(order.Amount)) generates "SELECT SUM(amount)"
// Example: def.Filter(def.Sum[float64](order.Amount) >= minAmount) generates "WHERE SUM(amount) >= ${minAmount}"
func Sum[T any](any) T { var zero T; return zero }

// Avg creates an AVG aggregate function.
// Example: def.Column(def.Avg(product.Price)) generates "SELECT AVG(price)"
func Avg[T any](any) T { var zero T; return zero }

// Max creates a MAX aggregate function.
// Example: def.Column(def.Max(user.Age)) generates "SELECT MAX(age)"
func Max[T any](any) T { var zero T; return zero }

// Min creates a MIN aggregate function.
// Example: def.Column(def.Min(user.Age)) generates "SELECT MIN(age)"
func Min[T any](any) T { var zero T; return zero }

// Func creates a custom SQL function call for database-specific functions.
// The first argument is the function name, followed by the function arguments.
// Example: def.Func("DATE_FORMAT", user.CreatedAt, "%Y-%m-%d")
// generates "DATE_FORMAT(created_at, '%Y-%m-%d')"
// Example: def.Filter(def.Func[string]("COALESCE", user.Name, "Unknown") != "Unknown")
// generates "WHERE COALESCE(name, 'Unknown') != 'Unknown'"
func Func[T any](string, ...any) T { var zero T; return zero }

// Create defines an INSERT operation.
// It can take either a struct pointer (entity mode) or Set expressions (field mode).
// Entity mode: def.Create(user)
//
//	generates "INSERT INTO users (name, age) VALUES (${user.Name}, ${user.Age})"
//
// Field mode: def.Create(def.Set(user.Name, name), def.Set(user.Age, age))
//
//	generates "INSERT INTO users (name, age) VALUES (${name}, ${age})"
func Create(...any) any { return nil }

// Update defines an UPDATE operation.
// It can take either a struct pointer (entity mode) or Set expressions (field mode).
// Entity mode: def.Update(user, def.Filter(user.ID == user.ID))
//
//	generates "UPDATE users SET name = ${user.Name}, age = ${user.Age} WHERE id = ${user.ID}"
//
// Field mode: def.Update(def.Set(user.Name, name), def.Filter(user.ID == id))
//
//	generates "UPDATE users SET name = ${name} WHERE id = ${id}"
func Update(any, ...any) any { return nil }

// Delete defines a DELETE operation.
// It requires at least one Filter expression to prevent accidental full table deletion.
// Example: def.Delete(def.Filter(user.ID == id)) generates "DELETE FROM users WHERE id = ${id}"
func Delete(any, ...any) any { return nil }

// Set specifies a column assignment for Create or Update operations.
// First argument is the field reference (e.g., user.Name), second is the value.
// Example: def.Set(user.Name, name) generates "name = ${name}"
// Example: def.Set(user.Count, user.Count+1) generates "count = count + 1"
// Example: def.Set(user.UpdatedAt, def.Func[any]("now")) generates "updated_at = now()"
func Set(any, any) any { return nil }

// Limit specifies the maximum number of rows to return.
// Example: def.Limit(10) generates "LIMIT 10"
// Example: def.Limit(pageSize) generates "LIMIT ${pageSize}"
func Limit(int) any { return nil }

// Offset specifies the number of rows to skip before returning results.
// Example: def.Offset(20) generates "OFFSET 20"
// Example: def.Offset(offset) generates "OFFSET ${offset}"
func Offset(int) any { return nil }
