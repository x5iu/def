package defgen

import (
	"go/token"
	"go/types"
	"strings"
	"testing"
)

func newSQLTestPackage() (*Package, *types.Named) {
	userType := namedStructType("example.com/demo", "demo", "User", []namedField{
		{name: "ID", typ: types.Typ[types.Int64], tag: `db:"id" primary_key:"true"`},
		{name: "Name", typ: types.Typ[types.String], tag: `db:"name"`},
	})
	pkg := &Package{Tables: map[string]*TableBinding{}}
	addTableBindingFromNamed(pkg, userType, "users")
	return pkg, userType
}

func TestGenerateSQLAndMutationSQLErrorBranches(t *testing.T) {
	pkg, userType := newSQLTestPackage()
	userKey := getTypeKey(userType)

	_, err := GenerateSQL(pkg, &QueryMethod{
		Name: "BadFilter",
		ReturnType: ReturnTypeInfo{
			Type: userType,
		},
		Filters: []*FilterExpr{
			{
				Kind: FilterComparison,
				Op:   token.EQL,
				Left: FilterOperand{
					IsField:   true,
					FieldPath: []FieldPathElement{{VarName: "user", Type: userType}},
				},
				Right: FilterOperand{IsParam: true, ParamName: "id"},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid field path") {
		t.Fatalf("GenerateSQL(bad filter) error = %v, want invalid field path", err)
	}

	sql, err := GenerateSQL(pkg, &QueryMethod{
		Name: "NilFilter",
		ReturnType: ReturnTypeInfo{
			Type: userType,
		},
		Filters: []*FilterExpr{nil},
	})
	if err != nil || !strings.Contains(sql, "SELECT * FROM users") {
		t.Fatalf("GenerateSQL(nil filter) = (%q, %v), want select users", sql, err)
	}

	_, err = generateWithClausesSQL(pkg, []WithClause{
		{Name: "x", TargetType: "missing"},
	})
	if err == nil || !strings.Contains(err.Error(), "could not determine source table") {
		t.Fatalf("generateWithClausesSQL() error = %v, want source table error", err)
	}

	_, err = buildFilterConditions(pkg, []*FilterExpr{
		{
			Kind: FilterComparison,
			Op:   token.EQL,
			Left: FilterOperand{
				IsField:   true,
				FieldPath: []FieldPathElement{{VarName: "user", Type: userType}},
			},
			Right: FilterOperand{IsParam: true, ParamName: "id"},
		},
	}, nil)
	if err == nil {
		t.Fatalf("buildFilterConditions() expected analyze error")
	}

	_, err = generateUpdateSQL(pkg, &MutationMethod{
		Name:       "BadUpdateSet",
		Kind:       MethodKindUpdate,
		TargetType: userKey,
		Sets: []SetExpr{
			{
				FieldPath: []FieldPathElement{
					{VarName: "user", Type: userType},
					{FieldName: "Missing"},
				},
				Value: SetValue{IsParam: true, ParamName: "name"},
			},
		},
		Filters: []*FilterExpr{
			{
				Kind: FilterComparison,
				Op:   token.EQL,
				Left: FilterOperand{
					IsField: true,
					FieldPath: []FieldPathElement{
						{VarName: "user", Type: userType},
						{FieldName: "ID"},
					},
				},
				Right: FilterOperand{IsParam: true, ParamName: "id"},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "no columns specified for UPDATE") {
		t.Fatalf("generateUpdateSQL(no columns) error = %v, want no columns", err)
	}

	_, err = generateDeleteSQL(pkg, &MutationMethod{
		Name:       "BadDeleteFilter",
		Kind:       MethodKindDelete,
		TargetType: userKey,
		Filters: []*FilterExpr{
			{
				Kind: FilterComparison,
				Op:   token.EQL,
				Left: FilterOperand{
					IsField:   true,
					FieldPath: []FieldPathElement{{VarName: "user", Type: userType}},
				},
				Right: FilterOperand{IsParam: true, ParamName: "id"},
			},
		},
	})
	if err == nil {
		t.Fatalf("generateDeleteSQL() expected analyze error")
	}
}

func TestFormatAndParserEdgeBranches(t *testing.T) {
	if got := FormatSQL(""); got != "" {
		t.Fatalf("FormatSQL(empty) = %q, want empty", got)
	}

	if got := formatSelectSQL("SELECT 1"); got != "SELECT 1" {
		t.Fatalf("formatSelectSQL(no from) = %q, want unchanged", got)
	}

	if got := formatInsertSQL("INSERT INTO users (id)"); got != "INSERT INTO users (id)" {
		t.Fatalf("formatInsertSQL(no values) = %q, want unchanged", got)
	}

	rawInsert := "INSERT INTO users VALUES (${id})"
	if got := formatInsertSQL(rawInsert); got != rawInsert {
		t.Fatalf("formatInsertSQL(no columns) = %q, want unchanged", got)
	}

	rawUpdate := "UPDATE users RETURNING id"
	if got := formatUpdateSQL(rawUpdate); got != rawUpdate {
		t.Fatalf("formatUpdateSQL(no set) = %q, want unchanged", got)
	}

	if got := formatSetClause("name = ${name}"); got != "name = ${name}" {
		t.Fatalf("formatSetClause(non set) = %q", got)
	}
	if got := formatSetClause("SET   "); got != "SET" {
		t.Fatalf("formatSetClause(empty set) = %q, want SET", got)
	}

	withSelect := formatWithSQL("WITH c AS (SELECT 1) SELECT id FROM users")
	if !strings.Contains(withSelect, "SELECT id") {
		t.Fatalf("formatWithSQL(select main) = %q", withSelect)
	}
	withDelete := formatWithSQL("WITH c AS (SELECT 1) DELETE FROM users")
	if !strings.Contains(withDelete, "DELETE FROM users") {
		t.Fatalf("formatWithSQL(delete main) = %q", withDelete)
	}
	withInsert := formatWithSQL("WITH c AS (SELECT 1) INSERT INTO users (id) VALUES (${id})")
	if !strings.Contains(withInsert, "INSERT INTO users") {
		t.Fatalf("formatWithSQL(insert main) = %q", withInsert)
	}

	if q, v := parseFromSourceQualifier(""); q != "" || v != "" {
		t.Fatalf("parseFromSourceQualifier(empty) = (%q,%q), want empty", q, v)
	}
	if q, v := parseFromSourceQualifier("due d"); q != "due" || v != "d" {
		t.Fatalf("parseFromSourceQualifier(alias) = (%q,%q), want (due,d)", q, v)
	}
	if q, v := parseFromSourceQualifier("schema.tasks"); q != "schema.tasks" || v != "tasks" {
		t.Fatalf("parseFromSourceQualifier(schema) = (%q,%q), want (schema.tasks,tasks)", q, v)
	}

	parts := splitConditions(" AND name = 'a''b' OR id = ${id}")
	if len(parts) < 2 {
		t.Fatalf("splitConditions() unexpected parts: %+v", parts)
	}
}

func TestResolveSetColumnNameBranches(t *testing.T) {
	pkg, userType := newSQLTestPackage()

	if got := resolveSetColumnName(pkg, SetExpr{}); got != "" {
		t.Fatalf("resolveSetColumnName(empty) = %q, want empty", got)
	}
	if got := resolveSetColumnName(pkg, SetExpr{
		FieldPath: []FieldPathElement{
			{VarName: "user", Type: userType},
			{FieldName: "Missing"},
		},
	}); got != "" {
		t.Fatalf("resolveSetColumnName(missing field) = %q, want empty", got)
	}

	if got := resolveSetColumnName(pkg, SetExpr{
		FieldPath: []FieldPathElement{
			{VarName: "other", Type: types.Typ[types.Int]},
			{FieldName: "ID"},
		},
	}); got != "" {
		t.Fatalf("resolveSetColumnName(unbound type) = %q, want empty", got)
	}
}

func TestGenerateSQLLiteralPaginationAndDeleteNilFilterBranches(t *testing.T) {
	pkg, userType := newSQLTestPackage()
	key := getTypeKey(userType)

	sql, err := GenerateSQL(pkg, &QueryMethod{
		Name: "LiteralPagination",
		ReturnType: ReturnTypeInfo{
			Type: userType,
		},
		Limit:  &PaginationExpr{Value: 5},
		Offset: &PaginationExpr{Value: 10},
	})
	if err != nil || !strings.Contains(sql, "LIMIT 5") || !strings.Contains(sql, "OFFSET 10") {
		t.Fatalf("GenerateSQL(literal pagination) = (%q, %v)", sql, err)
	}

	_, err = generateDeleteSQL(pkg, &MutationMethod{
		Name:       "MissingTarget",
		Kind:       MethodKindDelete,
		TargetType: "missing",
		Filters:    []*FilterExpr{{Kind: FilterAnd}},
	})
	if err == nil || !strings.Contains(err.Error(), "could not determine table") {
		t.Fatalf("generateDeleteSQL(missing target) error = %v", err)
	}

	delSQL, err := generateDeleteSQL(pkg, &MutationMethod{
		Name:       "NilFilter",
		Kind:       MethodKindDelete,
		TargetType: key,
		Filters:    []*FilterExpr{nil},
	})
	if err != nil || delSQL != "DELETE FROM users" {
		t.Fatalf("generateDeleteSQL(nil analyzed filter) = (%q, %v)", delSQL, err)
	}
}
