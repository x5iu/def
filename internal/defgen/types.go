package defgen

import (
	"go/ast"
	"go/token"
	"go/types"
)

// FieldInfo represents a struct field with its db tag mapping.
type FieldInfo struct {
	GoName       string // Go field name, e.g., "ID"
	DBName       string // Database column name from db tag, e.g., "id"
	Type         types.Type
	IsPrimaryKey    bool // true if this field has primary_key:"true" tag
	IsAutoIncrement bool // true if this field has auto_increment:"true" tag
}

// ForeignKeyInfo represents a foreign key relationship.
type ForeignKeyInfo struct {
	FieldName string     // Go field name, e.g., "User"
	KeyColumn string     // Foreign key column, e.g., "user_id"
	RefType   types.Type // Referenced type, e.g., *User
	Inverse   string     // Custom has-many field name on the referenced table, e.g., "Endpoints"
}

// TableBinding represents a type bound to a database table.
type TableBinding struct {
	Type        types.Type
	TypeName    string           // e.g., "User"
	TableName   string           // e.g., "users"
	Fields      []FieldInfo      // db tag fields
	ForeignKeys []ForeignKeyInfo // foreign_key tag fields
	PrimaryKey  *FieldInfo       // primary key field (marked with primary_key:"true")
}

// ParamInfo represents a method parameter.
type ParamInfo struct {
	Name string
	Type types.Type
}

// ReturnTypeInfo represents the return type of a query method.
type ReturnTypeInfo struct {
	Type       types.Type
	IsSlice    bool // true for []*T, false for *T
	ElemType   types.Type
	StructName string // e.g., "User" or "Project"
}

// QueryMethod represents a query method definition.
type QueryMethod struct {
	Name        string
	Receiver    string // Receiver type name
	Params      []ParamInfo
	ParamTypes  []types.Type // All parameter types in declaration order (including context.Context)
	ReturnType  ReturnTypeInfo
	ResultTypes []types.Type // All result types in declaration order
	Columns     []ColumnExpr // SELECT columns, empty means SELECT *
	Filters     []*FilterExpr
	Limit       *PaginationExpr // LIMIT expression, nil means no LIMIT
	Offset      *PaginationExpr // OFFSET expression, nil means no OFFSET
	Pos         token.Pos
}

// FilterOperand represents one side of a filter expression.
type FilterOperand struct {
	IsParam   bool   // true if this is a parameter reference
	IsLiteral bool   // true if this is a literal value
	IsField   bool   // true if this is a field access
	IsFunc    bool   // true if this is a function call
	IsNil     bool   // true if this is a nil literal
	ParamName string // parameter name if IsParam
	// Field access path
	FieldPath []FieldPathElement
	// Literal value
	LiteralValue string
	LiteralKind  token.Token // STRING, INT, FLOAT
	// Function call
	FuncName string    // Function name (COUNT, SUM, COALESCE, etc.)
	FuncArgs []FuncArg // Function arguments
}

// FieldPathElement represents one element in a field access path.
type FieldPathElement struct {
	VarName      string // Variable name (only for first element)
	FieldName    string // Field name
	IsForeignKey bool   // true if this field is a foreign key
	Type         types.Type
}

// FilterKind represents the type of a filter expression node.
type FilterKind int

const (
	FilterComparison FilterKind = iota // Leaf: a == b
	FilterIn                           // Leaf: def.In(a, b)
	FilterAnd                          // Internal: expr && expr
	FilterOr                           // Internal: expr || expr
)

// MethodKind represents the type of operation (query or mutation).
type MethodKind int

const (
	MethodKindQuery  MethodKind = iota // SELECT
	MethodKindCreate                   // INSERT
	MethodKindUpdate                   // UPDATE
	MethodKindDelete                   // DELETE
)

// FilterExpr represents a filter expression tree node.
type FilterExpr struct {
	Kind FilterKind

	// For comparison nodes (Kind == FilterComparison or FilterIn)
	Left  FilterOperand
	Op    token.Token // ==, !=, <, >, <=, >=
	Right FilterOperand

	// For boolean combination nodes (Kind == FilterAnd or FilterOr)
	Children []*FilterExpr

	Pos token.Pos
}

// FuncArg represents an argument to a SQL function.
type FuncArg struct {
	IsField   bool               // true if this is a field reference
	IsLiteral bool               // true if this is a literal value
	IsParam   bool               // true if this is a method parameter
	FieldPath []FieldPathElement // Field path if IsField
	Value     string             // Literal value or param name
	Kind      token.Token        // Literal kind (STRING, INT, etc.)
}

// ColumnExpr represents a SELECT column expression.
type ColumnExpr struct {
	IsFunc    bool               // true if this is a function call
	FuncName  string             // Function name: COUNT, SUM, DATE_FORMAT, etc.
	FuncArgs  []FuncArg          // Function arguments
	FieldPath []FieldPathElement // Field path if not a function (IsFunc=false)
}

// PaginationExpr represents a LIMIT or OFFSET expression.
type PaginationExpr struct {
	IsParam   bool   // true if using a method parameter
	ParamName string // parameter name if IsParam is true
	Value     int64  // literal value if IsParam is false
}

// OrderByExpr represents one ORDER BY item.
type OrderByExpr struct {
	Column ColumnExpr // Column or function expression
	Desc   bool       // true for DESC, false for ASC
}

// WithClause represents a WITH (CTE) query used by mutation statements.
type WithClause struct {
	Name string // CTE name, e.g., "due"

	TargetType string // source table type key
	Columns    []ColumnExpr
	Filters    []*FilterExpr
	OrderBy    []OrderByExpr
	Limit      *PaginationExpr
	Offset     *PaginationExpr

	ForUpdateSkipLocked bool // PostgreSQL FOR UPDATE SKIP LOCKED
}

// SetExpr represents a SET clause assignment in INSERT/UPDATE operations.
type SetExpr struct {
	FieldPath []FieldPathElement // Field path (e.g., user.Name -> [user, Name])
	Value     SetValue           // The value to assign
}

// SetValue represents the value in a SET assignment.
type SetValue struct {
	IsParam      bool        // true if this is a method parameter reference
	IsLiteral    bool        // true if this is a literal value
	ParamName    string      // parameter name if IsParam
	LiteralValue string      // literal value if IsLiteral
	LiteralKind  token.Token // STRING, INT, FLOAT for literals
	ExprSQL      string      // preformatted SQL expression (e.g., "count + 1", "now()")
}

// MutationMethod represents a mutation method (INSERT/UPDATE/DELETE).
type MutationMethod struct {
	Kind          MethodKind          // Create, Update, or Delete
	Name          string              // Method name
	Receiver      string              // Receiver type name
	Params        []ParamInfo         // Method parameters
	ParamTypes    []types.Type        // All parameter types in declaration order (including context.Context)
	ResultTypes   []types.Type        // All result types in declaration order
	TargetType    string              // Target type key (pkgpath.Type)
	Sets          []SetExpr           // SET expressions for Create/Update
	Filters       []*FilterExpr       // WHERE conditions for Update/Delete
	EntityParam   *ParamInfo          // Entity parameter for Create/Update entity mode
	Pos           token.Pos           // Position in source
	ReturnType    *MutationReturnType // Return type info (nil means sql.Result)
	ReturningCols []ColumnExpr        // postgres.Returning() specified columns (empty with ReturnType means RETURNING *)

	// PostgreSQL ON CONFLICT support
	ConflictColumns []ColumnExpr // ON CONFLICT (col1, col2) target columns
	ConflictAction  string       // "nothing" or "update"
	ConflictSets    []SetExpr    // DO UPDATE SET assignments (only when ConflictAction == "update")

	// UPDATE extensions
	WithClauses []WithClause // WITH cte AS (...)
	FromSources []string     // UPDATE ... FROM source1, source2
}

// MutationReturnType represents the return type of a mutation method.
type MutationReturnType struct {
	Type       types.Type // The actual return type
	IsSlice    bool       // Whether it returns a slice
	StructName string     // Struct name (e.g., "User")
	IsScalar   bool       // Whether it's a scalar type (int64, string, etc.)
}

// Package represents a parsed package with all def-related information.
type Package struct {
	Fset    *token.FileSet
	PkgPath string
	PkgName string
	Dir     string

	Tables          map[string]*TableBinding // TypeName -> TableBinding
	Methods         []*QueryMethod
	MutationMethods []*MutationMethod         // INSERT/UPDATE/DELETE methods
	Interfaces      map[string]*InterfaceInfo // Interface name -> InterfaceInfo
	TypesInfo       *types.Info
	Syntax          []*ast.File
	TypesPkg        *types.Package

	// Relation-related generated content
	RelationMethods  []*RelationMethod
	CallbackMethods  []*CallbackMethod
	SliceTypeAliases []*SliceTypeAlias
}

// InterfaceInfo represents an interface definition found in the source.
type InterfaceInfo struct {
	Name    string
	Methods []InterfaceMethod
	Pos     token.Pos
}

// InterfaceMethod represents a method in an interface.
type InterfaceMethod struct {
	Name        string
	Signature   string // Full signature for output
	Params      []ParamInfo
	ParamTypes  []types.Type // All parameter types in declaration order
	ResultTypes []types.Type // All result types in declaration order
	ReturnType  ReturnTypeInfo
}

// GeneratedMethod represents a method to be generated with SQL comment.
type GeneratedMethod struct {
	Name      string
	Signature string
	SQL       string
}

// RelationMethod represents a private method for loading related data.
type RelationMethod struct {
	MethodName   string     // e.g., "getUserByID" or "getProjectsByUserID"
	ParamName    string     // e.g., "id" or "userID"
	ParamType    types.Type // e.g., int64
	RefType      types.Type // e.g., *User or []*Project
	RefTypeName  string     // e.g., "User" or "Project"
	RefTableName string     // e.g., "users" or "projects"
	WhereColumn  string     // e.g., "id" or "user_id"
	IsSlice      bool       // true for one-to-many
}

// CallbackMethod represents a Callback implementation for a struct.
type CallbackMethod struct {
	StructName     string          // e.g., "Project"
	StructTypeName string          // e.g., "*Project"
	Fields         []CallbackField // fields to populate
	IDField        *FieldInfo      // primary key field for caching
}

// CallbackField represents a field to populate in Callback.
type CallbackField struct {
	FieldName     string // e.g., "User"
	MethodName    string // e.g., "getUserByID"
	KeyFieldName  string // e.g., "UserID"
	IsSlice       bool   // true for one-to-many
	CacheKey      string // e.g., "user_id" for building cache key
	RefTypeName   string // actual Go type name from BindTable (e.g., "User" for field "Author *User")
	AliasTypeName string // slice type alias name for cache (e.g., "ModelEndpointRows"); only set for has-many

	// For slice fields (has-many), SliceType is the underlying slice type (e.g., "[]*Project").
	// FieldIsAlias indicates whether the struct field type is the generated alias type (e.g., "Projects").
	SliceType    string
	FieldIsAlias bool
}

// SliceTypeAlias represents a slice type alias for Callback support.
type SliceTypeAlias struct {
	AliasName string // e.g., "Projects"
	ElemType  string // e.g., "*Project"
}
