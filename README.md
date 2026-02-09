# def - SQL Query Code Generator

A code generation tool similar to Google Wire that scans Go code with `def.Query`, `def.Create`, `def.Update`, `def.Delete` definitions and generates interface definitions with SQL comments.

Supports:
- **Query**: Column selection, aggregate functions (COUNT, SUM, AVG, MAX, MIN), custom SQL functions via `def.Func`
- **Mutations**: INSERT, UPDATE, DELETE operations via `def.Create`, `def.Update`, `def.Delete`

## Installation

```bash
go install github.com/x5iu/def/cmd/def@latest
```

## Usage

```bash
# Generate code for current package
def generate .

# Generate code for specific package
def generate ./path/to/package

# Generate code for all packages
def generate ./...

# Short alias
def gen ./...
```

When the pattern matches multiple packages (e.g. `./...`), `def` generates one output file per package directory.

If `-o/--output` is provided, it is treated as a file name relative to each package directory (absolute paths are only supported for single-package patterns).

### CLI Options

| Option | Short | Description |
|--------|-------|-------------|
| `--output` | `-o` | Output file path (default: `def_gen.go` in package directory) |
| `--tags` | | Build tags for parsing source files |
| `--interface` | `-T` | Name of the generated interface |
| `--defc` | | Custom defc command for `//go:generate` directive (default: `go run -mod=mod github.com/x5iu/defc@latest`) |
| `--defc-features` | | Additional defc features to include (e.g., `sqlx/rebind,sqlx/in`). Merged with auto-detected features like `sqlx/callback`. |
| `--defc-generate` | | Directly invoke defc to generate the implementation file, instead of emitting a `//go:generate` directive. When set, `--defc` is ignored. |

**Examples:**

```bash
# Custom output file
def generate -o query_gen.go .

# Include files with build tags
def generate --tags def .
# This will include files marked with //go:build def

# Combine options
def generate -o query_gen.go --tags def .

# Customize defc command in generated //go:generate directive
def generate --defc "go tool defc" .
def generate --defc "go run -mod=mod github.com/x5iu/defc@v0.5.0" .
def generate --defc /usr/local/bin/defc .

# Specify additional defc features
def generate --defc-features "sqlx/rebind,sqlx/in" .

# Generate intermediate + implementation in one step (no //go:generate directive)
def generate --tags def --defc-generate --defc-features "sqlx/rebind,sqlx/in" -o store.go .
```

## Input Format Specification

### Struct Definitions

Define database table structures using the `db` tag for column mapping:

```go
type User struct {
    ID   int64  `db:"id"`
    Name string `db:"name"`
    Age  uint8  `db:"age"`

    // Non-database fields use db:"-"
    Projects []*Project `db:"-"`
}
```

### Foreign Key Relationships

Use `foreign_key` tag to define relationships between tables:

```go
type Project struct {
    ID     int64  `db:"id"`
    Name   string `db:"name"`
    UserID int64  `db:"user_id"`
    Status string `db:"status"`

    // Foreign key: User field references User table via user_id column
    User *User `db:"-" foreign_key:"user_id"`
}
```

The `foreign_key` tag specifies which column in the current table is used as the foreign key.

#### Custom Has-Many Field Name (`inverse` tag)

When generating Callback methods, `def` automatically creates has-many relations by looking for a field named `<TypeName>s` on the referenced table (e.g., `ProjectRows` on `User`). If your field has a different name, use the `inverse` tag to specify the actual field name:

```go
type ModelEndpointRow struct {
    ID      int64 `db:"id"`
    ModelID int64 `db:"model_id"`

    // inverse: the has-many field on ModelRow is named "Endpoints", not "ModelEndpointRows"
    Model *ModelRow `db:"-" foreign_key:"model_id" inverse:"Endpoints"`
}

type ModelRow struct {
    ID int64 `db:"id"`

    // This field will be populated by the generated Callback
    Endpoints []*ModelEndpointRow `db:"-"`
}
```

Without `inverse:"Endpoints"`, `def` would look for a field named `ModelEndpointRows` on `ModelRow` and fail to generate the has-many callback.

### Table Binding

Bind Go types to database table names using `def.Init` and `def.BindTable`:

```go
func init() {
    def.Init(
        def.BindTable[User]("users"),
        def.BindTable[Project]("projects"),
    )
}
```

### Query Definition

Define query methods using `def.Query` and `def.Filter`:

```go
type querier struct{}

func (q *querier) GetUserByID(ctx context.Context, id int64) (user *User, err error) {
    def.Query(
        def.Filter(user.ID == id),
    )
    return nil, nil
}
```

### Filter Expressions

`def.Filter` accepts a single boolean expression. Supported expression types:

| Expression Type | Example | SQL Output |
|----------------|---------|------------|
| Parameter reference | `user.ID == id` | `id = ${id}` |
| String literal | `user.Status == "active"` | `status = 'active'` |
| Number literal | `user.Age == 18` | `age = 18` |
| Not equal | `user.Status != "deleted"` | `status != 'deleted'` |
| Greater than | `user.Age > 18` | `age > 18` |
| Less than | `user.Age < 60` | `age < 60` |
| Greater or equal | `user.Age >= 18` | `age >= 18` |
| Less or equal | `user.Age <= 60` | `age <= 60` |
| IN query | `def.In(user.ID, ids)` | `id IN (${ids})` |
| AND condition | `a == b && c == d` | `(a = b AND c = d)` |
| OR condition | `a == b \|\| c == d` | `(a = b OR c = d)` |
| Foreign key access | `project.User.Name == username` | Subquery (see below) |
| Function call | `def.Count[int64](t.ID) > 0` | `COUNT(id) > 0` |

### Column Selection

Use `def.Column` to select specific columns instead of `SELECT *`:

```go
// Select single column
func (q *querier) GetUserName(ctx context.Context, id int64) (name string, err error) {
    var user User
    def.Query(
        def.Column(user.Name),
        def.Filter(user.ID == id),
    )
    return "", nil
}
// Generates: SELECT name FROM users WHERE id = ${id}

// Select multiple columns
func (q *querier) GetUserBasic(ctx context.Context, id int64) (name string, age int, err error) {
    var user User
    def.Query(
        def.Column(user.Name),
        def.Column(user.Age),
        def.Filter(user.ID == id),
    )
    return "", 0, nil
}
// Generates: SELECT name, age FROM users WHERE id = ${id}
```

### Pagination (LIMIT/OFFSET)

Use `def.Limit` and `def.Offset` for pagination:

```go
// With literal values
func (q *querier) GetTop10Users(ctx context.Context) ([]*User, error) {
    var user User
    def.Query(
        def.Limit(10),
    )
    return nil, nil
}
// Generates: SELECT * FROM users LIMIT 10

// With parameters
func (q *querier) GetUsersByPage(ctx context.Context, limit, offset int) ([]*User, error) {
    var user User
    def.Query(
        def.Limit(limit),
        def.Offset(offset),
    )
    return nil, nil
}
// Generates: SELECT * FROM users LIMIT ${limit} OFFSET ${offset}

// Combined with filter
func (q *querier) GetActiveUsersByPage(ctx context.Context, status string, limit, offset int) ([]*User, error) {
    var user User
    def.Query(
        def.Filter(user.Status == status),
        def.Limit(limit),
        def.Offset(offset),
    )
    return nil, nil
}
// Generates: SELECT * FROM users WHERE status = ${status} LIMIT ${limit} OFFSET ${offset}
```

### Aggregate Functions

Built-in aggregate functions: `def.Count`, `def.Sum`, `def.Avg`, `def.Max`, `def.Min`

#### In Column Selection

```go
// COUNT
func (q *querier) CountUsers(ctx context.Context) (count int64, err error) {
    var user User
    def.Query(
        def.Column(def.Count(user.ID)),
    )
    return 0, nil
}
// Generates: SELECT COUNT(id) FROM users

// COUNT with condition
func (q *querier) CountActiveUsers(ctx context.Context, status string) (count int64, err error) {
    var user User
    def.Query(
        def.Column(def.Count(user.ID)),
        def.Filter(user.Status == status),
    )
    return 0, nil
}
// Generates: SELECT COUNT(id) FROM users WHERE status = ${status}
```

#### In Filter (HAVING-like conditions)

Use generic type parameter to enable comparison operators:

```go
// Filter by COUNT
func (q *querier) GetUsersWithProjects(ctx context.Context) ([]*User, error) {
    var user User
    def.Query(
        def.Filter(def.Count[int64](user.ID) > 0),
    )
    return nil, nil
}
// Generates: SELECT * FROM users WHERE COUNT(id) > 0

// Filter by SUM
func (q *querier) GetHighValueOrders(ctx context.Context, minAmount float64) ([]*Order, error) {
    var order Order
    def.Query(
        def.Filter(def.Sum[float64](order.Amount) >= minAmount),
    )
    return nil, nil
}
// Generates: SELECT * FROM orders WHERE SUM(amount) >= ${minAmount}
```

### Custom SQL Functions

Use `def.Func` for database-specific functions that cannot be enumerated.

#### In Column Selection

```go
// MySQL DATE_FORMAT
func (q *querier) GetUserCreatedDate(ctx context.Context, id int64) (date string, err error) {
    var user User
    def.Query(
        def.Column(def.Func("DATE_FORMAT", user.CreatedAt, "%Y-%m-%d")),
        def.Filter(user.ID == id),
    )
    return "", nil
}
// Generates: SELECT DATE_FORMAT(created_at, '%Y-%m-%d') FROM users WHERE id = ${id}

// COALESCE
func (q *querier) GetUserNameOrDefault(ctx context.Context, id int64) (name string, err error) {
    var user User
    def.Query(
        def.Column(def.Func("COALESCE", user.Name, "Unknown")),
        def.Filter(user.ID == id),
    )
    return "", nil
}
// Generates: SELECT COALESCE(name, 'Unknown') FROM users WHERE id = ${id}
```

#### In Filter

Use generic type parameter to enable comparison operators:

```go
// COALESCE in filter
func (q *querier) GetUsersWithKnownName(ctx context.Context) ([]*User, error) {
    var user User
    def.Query(
        def.Filter(def.Func[string]("COALESCE", user.Name, "Unknown") != "Unknown"),
    )
    return nil, nil
}
// Generates: SELECT * FROM users WHERE COALESCE(name, 'Unknown') != 'Unknown'

// LENGTH in filter
func (q *querier) GetUsersWithLongName(ctx context.Context, minLen int) ([]*User, error) {
    var user User
    def.Query(
        def.Filter(def.Func[int]("LENGTH", user.Name) >= minLen),
    )
    return nil, nil
}
// Generates: SELECT * FROM users WHERE LENGTH(name) >= ${minLen}
```

`def.Func` arguments can be:
- Field references (`user.Name`) → converted to column name
- String literals → converted to SQL string with single quotes
- Number literals → used directly
- Method parameters → converted to `${param}`

### Mutation Operations

#### Create (INSERT)

Use `def.Create` to generate INSERT statements:

**Entity Mode** - Insert entire struct:

```go
func (q *querier) CreateUser(ctx context.Context, user *User) (sql.Result, error) {
    return def.Create(user)
}
// Generates:
// INSERT INTO users (
//     id,
//     name,
//     age
// ) VALUES (
//     ${user.ID},
//     ${user.Name},
//     ${user.Age}
// )
```

**Field Mode** - Insert specific columns:

```go
func (q *querier) CreateUser(ctx context.Context, name string, age int) (sql.Result, error) {
    var user User
    return def.Create(
        def.Set(user.Name, name),
        def.Set(user.Age, age),
    )
}
// Generates:
// INSERT INTO users (
//     name,
//     age
// ) VALUES (
//     ${name},
//     ${age}
// )
```

#### Update

Use `def.Update` with `def.Set` and optional `def.Filter`:

**Entity Mode** - Update entire struct (primary key excluded from SET):

```go
func (q *querier) UpdateUser(ctx context.Context, user *User) (sql.Result, error) {
    return def.Update(user, def.Filter(user.ID == user.ID))
}
// Generates: UPDATE users SET name = ${user.Name}, age = ${user.Age} WHERE id = ${user.ID}
```

**Field Mode** - Update specific columns:

```go
func (q *querier) UpdateUserName(ctx context.Context, id int64, name string) (sql.Result, error) {
    var user User
    return def.Update(
        def.Set(user.Name, name),
        def.Filter(user.ID == id),
    )
}
// Generates: UPDATE users SET name = ${name} WHERE id = ${id}
```

Multiple SET clauses:

```go
func (q *querier) UpdateUser(ctx context.Context, id int64, name string, age int) (sql.Result, error) {
    var user User
    return def.Update(
        def.Set(user.Name, name),
        def.Set(user.Age, age),
        def.Filter(user.ID == id),
    )
}
// Generates: UPDATE users SET name = ${name}, age = ${age} WHERE id = ${id}
```

#### Delete

Use `def.Delete` with optional `def.Filter`:

```go
func (q *querier) DeleteUser(ctx context.Context, id int64) (sql.Result, error) {
    var user User
    return def.Delete(
        def.Filter(user.ID == id),
    )
}
// Generates: DELETE FROM users WHERE id = ${id}
```

Delete with multiple conditions:

```go
func (q *querier) DeleteInactiveUsers(ctx context.Context, status string, maxAge int) (sql.Result, error) {
    var user User
    return def.Delete(
        def.Filter(user.Status == status),
        def.Filter(user.Age > maxAge),
    )
}
// Generates: DELETE FROM users WHERE status = ${status} AND age > ${maxAge}
```

**Note**: Delete requires at least one Filter argument to prevent accidental full table deletion (enforced at compile time).

## Output Format Specification

### Generated Interface

The tool generates an interface with SQL comments:

```go
package demo

import (
    "context"
    "database/sql"
)

type Querier interface {
    // GetUserByID query constbind
    /* SELECT * FROM users WHERE id = ${id} */
    GetUserByID(ctx context.Context, id int64) (*User, error)

    // GetProjectByUsername query constbind
    /* SELECT *
    FROM projects
    WHERE user_id IN (SELECT id FROM users WHERE name = ${username})
      AND status = 'active' */
    GetProjectByUsername(ctx context.Context, username string) ([]*Project, error)

    // CreateUser exec constbind
    /* INSERT INTO users (
        id,
        name,
        age
    ) VALUES (
        ${user.ID},
        ${user.Name},
        ${user.Age}
    ) */
    CreateUser(ctx context.Context, user *User) (sql.Result, error)

    // UpdateUserName exec constbind
    /* UPDATE users
    SET name = ${name}
    WHERE id = ${id} */
    UpdateUserName(ctx context.Context, id int64, name string) (sql.Result, error)

    // DeleteUser exec constbind
    /* DELETE FROM users
    WHERE id = ${id} */
    DeleteUser(ctx context.Context, id int64) (sql.Result, error)
}
```

### SQL Comment Format

Each method comment contains:
1. Method name followed by `query constbind` (for SELECT) or `exec constbind` (for INSERT/UPDATE/DELETE)
2. SQL statement

**Query methods**: `SELECT <columns> FROM <table> WHERE <conditions>`
- `<columns>` is `*` by default, or specific columns/functions if `def.Column` is used

**Mutation methods**:
- INSERT: `INSERT INTO <table> (<cols>) VALUES (<vals>)` (each column and value on its own line)
- UPDATE: `UPDATE <table> SET <assignments> WHERE <conditions>`
- DELETE: `DELETE FROM <table> WHERE <conditions>`

### Parameter Binding Format

Parameters are referenced using `${param}` syntax:
- Method parameter `id` becomes `${id}`
- Method parameter `username` becomes `${username}`

## SQL Generation Rules

### Simple Field Access

Direct field access is converted to column name:
```go
user.ID == id  →  id = ${id}
```

### Literal Values

- String literals: wrapped in single quotes `'value'`
- Number literals: used directly
- Boolean: not yet supported

### Comparison Operators

| Go Operator | SQL Operator |
|-------------|--------------|
| `==` | `=` |
| `!=` | `!=` |
| `<` | `<` |
| `>` | `>` |
| `<=` | `<=` |
| `>=` | `>=` |

### Boolean Expressions (AND / OR)

Use `&&` and `||` operators for combining conditions:

```go
// AND condition
def.Filter(project.Status == "active" && project.UserID == id)
// → (status = 'active' AND user_id = ${id})

// OR condition
def.Filter(project.Status == "active" || project.Status == "pending")
// → (status = 'active' OR status = 'pending')

// Complex nested conditions with parentheses
def.Filter((project.Status == "active" && project.UserID == id) || project.Status == "pending")
// → ((status = 'active' AND user_id = ${id}) OR status = 'pending')

// Combined with IN query
def.Filter(def.In(user.ID, ids) && user.Status == "active")
// → (id IN (${ids}) AND status = 'active')
```

### IN Query

Use `def.In` to generate IN queries:

```go
func (q *querier) GetUsersByIDs(ctx context.Context, ids []int64) ([]*User, error) {
    var user User
    def.Query(
        def.Filter(
            def.In(user.ID, ids),
        ),
    )
    return nil, nil
}
```

Generates:
```sql
SELECT * FROM users WHERE id IN (${ids})
```

### Foreign Key Subquery

When accessing a field through a foreign key relationship:

```go
project.User.Name == username
```

Generates a subquery:
```sql
user_id IN (SELECT id FROM users WHERE name = ${username})
```

Where:
- `users` is the table bound to `User` type
- `id` is assumed to be the primary key of the referenced table
- `name` is the column for `User.Name`
- `user_id` is the foreign key column specified in `foreign_key` tag

## Complete Example

### Input (demo.go)

```go
package demo

import (
    "context"
    "database/sql"

    "github.com/x5iu/def"
)

type User struct {
    ID   int64  `db:"id"`
    Name string `db:"name"`
    Age  uint8  `db:"age"`
    Projects []*Project `db:"-"`
}

type Project struct {
    ID     int64  `db:"id"`
    Name   string `db:"name"`
    UserID int64  `db:"user_id"`
    Status string `db:"status"`
    User *User `db:"-" foreign_key:"user_id"`
}

type Querier interface {
    GetUserByID(ctx context.Context, id int64) (*User, error)
    GetProjectsByUsername(ctx context.Context, username string) ([]*Project, error)
    CreateUser(ctx context.Context, user *User) (sql.Result, error)
    UpdateUserName(ctx context.Context, id int64, name string) (sql.Result, error)
    DeleteUser(ctx context.Context, id int64) (sql.Result, error)
}

func init() {
    def.Init(
        def.BindTable[User]("users"),
        def.BindTable[Project]("projects"),
    )
}

type querier struct{}

func (q *querier) GetUserByID(ctx context.Context, id int64) (user *User, err error) {
    def.Query(
        def.Filter(user.ID == id),
    )
    return nil, nil
}

func (q *querier) GetProjectByUsername(ctx context.Context, username string) ([]*Project, error) {
    var project Project
    def.Query(
        def.Filter(project.User.Name == username && project.Status == "active"),
    )
    return nil, nil
}

func (q *querier) CreateUser(ctx context.Context, user *User) (sql.Result, error) {
    return def.Create(user)
}

func (q *querier) UpdateUserName(ctx context.Context, id int64, name string) (sql.Result, error) {
    var user User
    return def.Update(
        def.Set(user.Name, name),
        def.Filter(user.ID == id),
    )
}

func (q *querier) DeleteUser(ctx context.Context, id int64) (sql.Result, error) {
    var user User
    return def.Delete(
        def.Filter(user.ID == id),
    )
}
```

### Output (def_gen.go)

```go
package demo

import (
    "context"
    "database/sql"
)

type Querier interface {
    // GetUserByID query constbind
    /* SELECT *
    FROM users
    WHERE id = ${id} */
    GetUserByID(ctx context.Context, id int64) (*User, error)

    // GetProjectByUsername query constbind
    /* SELECT *
    FROM projects
    WHERE user_id IN (SELECT id FROM users WHERE name = ${username})
      AND status = 'active' */
    GetProjectByUsername(ctx context.Context, username string) ([]*Project, error)

    // CreateUser exec constbind
    /* INSERT INTO users (
        id,
        name,
        age
    ) VALUES (
        ${user.ID},
        ${user.Name},
        ${user.Age}
    ) */
    CreateUser(ctx context.Context, user *User) (sql.Result, error)

    // UpdateUserName exec constbind
    /* UPDATE users
    SET name = ${name}
    WHERE id = ${id} */
    UpdateUserName(ctx context.Context, id int64, name string) (sql.Result, error)

    // DeleteUser exec constbind
    /* DELETE FROM users
    WHERE id = ${id} */
    DeleteUser(ctx context.Context, id int64) (sql.Result, error)
}
```

## Architecture

```
github.com/x5iu/def/
├── def.go                    # Placeholder API
├── go.mod
├── cmd/
│   └── def/
│       └── main.go           # CLI entry point
└── internal/
    └── defgen/
        ├── types.go          # Core data structures
        ├── schema.go         # Struct definition parsing
        ├── parse.go          # def.Init/Query parsing
        ├── analyze.go        # Expression analysis
        ├── relation.go       # Foreign key relation analysis & Callback generation
        ├── sql.go            # SQL generation
        └── codegen.go        # Code generation & defc integration
```

## License

MIT
