# def - SQL Query Code Generator

## Project Overview

`def` is a code generation tool similar to Google Wire. It scans Go code containing `def.Query`, `def.Create`, `def.Update`, `def.Delete` definitions and generates interface implementations with SQL comments.

### Key Features
- Parses struct definitions with `db`, `foreign_key`, and `primary_key` tags
- Reads table bindings from `def.Init` + `def.BindTable[T]("table")` calls
- Analyzes `def.Query` + `def.Filter` expressions to generate SQL SELECT statements
- Analyzes `def.Create`, `def.Update`, `def.Delete` + `def.Set`/`def.Filter` for mutation SQL
- Supports foreign key traversal for generating subqueries
- Generates Callback methods for automatic relation loading

## Execution Flow

### 1. Entry Point

**File**: `cmd/def/main.go`

- CLI uses `cobra` framework, defines `generate` command
- Calls `defgen.Generate(wd, pattern, opts)`
- Supports flags: `-o` (output file), `--tags` (build tags)

```
def generate [patterns...]
def generate ./...
def generate -o query_gen.go .
```

### 2. Package Loading

**File**: `internal/defgen/parse.go`
**Function**: `Load(wd, pattern string) (*Package, error)`

Uses `golang.org/x/tools/go/packages` to load Go packages with the following mode:
- `NeedName` - Package name
- `NeedFiles` - Source file paths
- `NeedSyntax` - Parsed AST
- `NeedTypes` - Type information
- `NeedTypesInfo` - Detailed type info (uses, defs)
- `NeedImports` - Import dependencies

Returns a `*Package` struct containing:
- `Fset` - File set for position info
- `Tables` - Map of type name to table binding
- `Interfaces` - Map of interface definitions
- `TypesInfo` - Type checker results
- `Syntax` - AST files

### 3. Code Scanning

**File**: `internal/defgen/parse.go`, `internal/defgen/schema.go`
**Function**: `Parse(pkg *Package) error`

Scanning happens in 5 steps:

#### Step 1: `parseAllStructs()`
- Scans all struct type declarations
- Parses `db` tags for column mappings
- Parses `primary_key` tags to identify primary key fields
- Parses `foreign_key` tags for relationships
- Stores in `map[string]*structInfo`

#### Step 2: `parseInterfaceDefs()`
- Scans interface type declarations
- Records method signatures
- Stores in `pkg.Interfaces`

#### Step 3: `parseTableBindings()`
- Finds `def.Init(...)` calls in source
- Parses each `def.BindTable[T]("table_name")` argument
- Maps Go types to database tables
- Stores in `pkg.Tables`

#### Step 4: `parseQueryMethods()`
- Finds methods containing `def.Query(...)` calls
- Parses method receiver, parameters, return type
- Extracts `def.Column(...)`, `def.Filter(...)`, `def.Limit(...)`, and `def.Offset(...)` from arguments
- Stores in `pkg.Methods`

#### Step 5: `parseMutationMethods()`
- Finds methods containing `def.Create(...)`, `def.Update(...)`, `def.Delete(...)` calls
- Parses method receiver, parameters
- Extracts `def.Set(...)` and `def.Filter(...)` from arguments
- Stores in `pkg.MutationMethods`

### 4. Expression Parsing

**File**: `internal/defgen/parse.go`

#### Column Expression: `parseColumnExpr()`
Supports:
- Field reference: `user.Name` → column lookup
- Aggregate functions: `def.Count(user.ID)`, `def.Sum(...)`, `def.Avg(...)`, `def.Max(...)`, `def.Min(...)`
- Custom SQL functions: `def.Func[T]("COALESCE", user.Name, "default")`

#### Filter Expression: `parseFilterExprRecursive()`
Recursively parses filter expressions into a tree:
- `&&` → `FilterAnd` node with children
- `||` → `FilterOr` node with children
- `==`, `!=`, `<`, `>`, `<=`, `>=` → `FilterComparison` leaf
- `def.In(field, values)` → `FilterIn` leaf
- Parentheses `(expr)` → recurse into inner expression

#### Field Path: `parseFieldPath()`
Parses selector expressions like `project.User.Name`:
- Builds path: `[project, User, Name]`
- Detects foreign key traversals
- Marks `IsForeignKey` on path elements

#### Set Expression: `parseSetExpr()`
Parses `def.Set(field, value)` calls for INSERT/UPDATE:
- First argument is a field selector (e.g., `user.Name`)
- Second argument is a parameter or literal value
- Returns `SetExpr` with field path and value

#### Pagination Expression: `parsePaginationExpr()`
Parses `def.Limit(n)` and `def.Offset(n)` calls:
- Supports integer literals: `def.Limit(10)` → `LIMIT 10`
- Supports parameter references: `def.Limit(pageSize)` → `LIMIT ${pageSize}`
- Returns `PaginationExpr` with value or parameter name

### 5. Filter Analysis

**File**: `internal/defgen/analyze.go`
**Function**: `AnalyzeFilter(pkg *Package, filter *FilterExpr) (*AnalyzedFilter, error)`

Transforms parsed filter tree into SQL-ready structure:

- `FilterAnd` → `AnalyzedFilterAnd` with children
- `FilterOr` → `AnalyzedFilterOr` with children
- `FilterComparison` / `FilterIn` → Analyzed leaf node with:
  - `ColumnName` - Database column
  - `Operator` - SQL operator
  - `Value` - Parameter placeholder `${param}` or literal

For foreign key traversals:
- Sets `IsSubquery = true`
- Stores `ForeignKeyCol`, `SubqueryTable`, `SubqueryColumn`, `SubqueryValue`

### 6. SQL Generation

**File**: `internal/defgen/sql.go`
**Function**: `GenerateSQL(pkg *Package, method *QueryMethod) (string, error)`

Generates complete SQL statement:

1. Determine table from return type
2. Build SELECT clause:
   - `*` if no columns specified
   - Column list from `def.Column()` expressions
3. Build WHERE clause:
   - Call `AnalyzeFilter()` for each filter
   - Call `formatFilterTree()` to produce SQL string
4. Build LIMIT/OFFSET clause:
   - Literal values: `LIMIT 10`
   - Parameter references: `LIMIT ${pageSize}`

**Function**: `formatFilterTree(filter *AnalyzedFilter) string`

Recursively formats filter tree:
- `AnalyzedFilterAnd` → `(cond1 AND cond2)`
- `AnalyzedFilterOr` → `(cond1 OR cond2)`
- `AnalyzedFilterIn` → `column IN (${param})`
- `AnalyzedFilterComparison` → `column = ${param}` or subquery

**Function**: `GenerateMutationSQL(pkg *Package, method *MutationMethod) (string, error)`

Generates mutation SQL statements:
- `MethodKindCreate` → `INSERT INTO table (col1, col2) VALUES (${val1}, ${val2})` (each column and value on its own line)
- `MethodKindUpdate` → `UPDATE table SET col1 = ${val1} WHERE condition`
- `MethodKindDelete` → `DELETE FROM table WHERE condition`

### 7. Relation Analysis

**File**: `internal/defgen/relation.go`
**Function**: `AnalyzeRelations(pkg *Package) error`

Analyzes foreign key relationships to generate:

1. **Belongs-to (many-to-one)**: `Project.User` → generates `getUserByID(id) (*User, error)`
2. **Has-many (one-to-many)**: Reverse inference → generates `getProjectsByUserID(userID) ([]*Project, error)`

Also generates:
- `CallbackMethod` - Auto-loading related data
- `SliceTypeAlias` - Type aliases for slice Callbacks (e.g., `type Projects []*Project`)

### 8. Code Generation

**File**: `internal/defgen/codegen.go`
**Function**: `generateCode(pkg *Package, opts *GenerateOptions) ([]byte, error)`

Generates the output file:

1. Build tags (if specified)
2. Package declaration
3. Import statements
4. Slice type aliases (for Callback)
5. Cache utilities (for avoiding circular references)
6. `//go:generate` directive for defc
7. Interface definition with:
   - Method signature
   - SQL comment annotation
8. Callback methods for structs

Output is formatted with `go/format`.

## Key Data Structures

**File**: `internal/defgen/types.go`

| Type | Description |
|------|-------------|
| `Package` | Parsed package with tables, methods, interfaces |
| `TableBinding` | Type-to-table mapping with fields and foreign keys |
| `QueryMethod` | Method definition with columns, filters, limit, and offset |
| `MutationMethod` | Method definition for INSERT/UPDATE/DELETE |
| `FilterExpr` | Filter expression tree node |
| `ColumnExpr` | SELECT column expression |
| `PaginationExpr` | LIMIT or OFFSET expression (literal or parameter) |
| `SetExpr` | SET clause assignment expression |
| `SetValue` | Value in a SET assignment |
| `FieldPathElement` | Element in field access path (e.g., `project.User.Name`) |
| `RelationMethod` | Generated relation query method |
| `CallbackMethod` | Generated Callback implementation |

## Struct Tags

The following struct tags are supported:

| Tag | Description | Example |
|-----|-------------|---------|
| `db` | Maps Go field to database column | `db:"id"` |
| `primary_key` | Marks field as primary key | `primary_key:"true"` |
| `foreign_key` | Defines foreign key relationship | `foreign_key:"user_id"` |

### Example

```go
type User struct {
    ID   int64  `db:"id" primary_key:"true"`
    Name string `db:"name"`
}

type Project struct {
    ID     int64  `db:"id" primary_key:"true"`
    Name   string `db:"name"`
    UserID int64  `db:"user_id"`
    User   *User  `db:"-" foreign_key:"user_id"`
}
```

**Note**: The `primary_key:"true"` tag is required for proper foreign key subquery generation and Callback method generation.

## Supported Expressions

### Filter Expressions
```go
def.Filter(user.ID == id)              // Simple comparison
def.Filter(user.Age > 18)              // Greater than
def.Filter(user.Status == "active")    // String literal
def.Filter(def.In(user.ID, ids))       // IN query
def.Filter(a && b)                     // AND
def.Filter(a || b)                     // OR
def.Filter((a && b) || c)              // Nested
def.Filter(project.User.Name == name)  // Foreign key (generates subquery)
```

### Column Expressions
```go
def.Column(user.Name)                           // Field reference
def.Column(def.Count(user.ID))                  // Aggregate
def.Column(def.Func[string]("COALESCE", user.Name, "default"))  // Custom function
```

### Pagination Expressions
```go
def.Limit(10)                                   // LIMIT 10
def.Limit(pageSize)                             // LIMIT ${pageSize}
def.Offset(20)                                  // OFFSET 20
def.Offset(offset)                              // OFFSET ${offset}
```

### Create Expressions (INSERT)
```go
// Entity mode - inserts entire struct
def.Create(user)                                // INSERT INTO users (
                                                //     id,
                                                //     name,
                                                //     age
                                                // ) VALUES (
                                                //     ${user.ID},
                                                //     ${user.Name},
                                                //     ${user.Age}
                                                // )

// Field mode - inserts specific columns
def.Create(                                     // INSERT INTO users (
    def.Set(user.Name, name),                   //     name,
    def.Set(user.Age, age),                     //     age
)                                               // ) VALUES (
                                                //     ${name},
                                                //     ${age}
                                                // )
```

### Update Expressions
```go
// Entity mode - updates entire struct (primary key excluded from SET)
def.Update(user, def.Filter(user.ID == user.ID))  // UPDATE users
                                                   // SET name = ${user.Name}, age = ${user.Age}
                                                   // WHERE id = ${user.ID}

// Field mode - updates specific columns
def.Update(                                     // UPDATE users
    def.Set(user.Name, name),                   // SET name = ${name}
    def.Filter(user.ID == id),                  // WHERE id = ${id}
)

def.Update(                                     // UPDATE users
    def.Set(user.Name, name),                   // SET name = ${name}, age = ${age}
    def.Set(user.Age, age),                     // WHERE id = ${id}
    def.Filter(user.ID == id),
)
```

### Delete Expressions
```go
def.Delete(def.Filter(user.ID == id))           // DELETE FROM users WHERE id = ${id}
def.Delete(                                     // DELETE FROM users
    def.Filter(user.Status == "inactive"),      // WHERE status = 'inactive' AND age > ${minAge}
    def.Filter(user.Age > minAge),
)
// Note: Delete requires at least one Filter argument (enforced at compile time)
```

## Generated Output Example

```go
//go:generate go run -mod=mod github.com/x5iu/defc generate -T UserRepository -o userrepository_impl.go

type UserRepository interface {
    // FindByID query constbind
    // SELECT *
    // FROM users
    // WHERE id = ${id}
    FindByID(ctx context.Context, id int64) (*User, error)

    // FindByStatus query constbind
    // SELECT *
    // FROM users
    // WHERE status = 'active'
    //   AND age > ${minAge}
    FindByStatus(ctx context.Context, minAge int) ([]*User, error)

    // FindByPage query constbind
    // SELECT *
    // FROM users
    // WHERE status = ${status}
    // LIMIT ${limit} OFFSET ${offset}
    FindByPage(ctx context.Context, status string, limit, offset int) ([]*User, error)

    // FindTop10 query constbind
    // SELECT *
    // FROM users
    // LIMIT 10
    FindTop10(ctx context.Context) ([]*User, error)

    // CreateUser exec constbind
    // INSERT INTO users (
    //     id,
    //     name,
    //     age
    // ) VALUES (
    //     ${user.ID},
    //     ${user.Name},
    //     ${user.Age}
    // )
    CreateUser(ctx context.Context, user *User) (sql.Result, error)

    // UpdateUserName exec constbind
    // UPDATE users
    // SET name = ${name}
    // WHERE id = ${id}
    UpdateUserName(ctx context.Context, id int64, name string) (sql.Result, error)

    // DeleteUser exec constbind
    // DELETE FROM users
    // WHERE id = ${id}
    DeleteUser(ctx context.Context, id int64) (sql.Result, error)
}
```
