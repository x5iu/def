package defgen

import (
	"go/token"
	"go/types"
	"testing"
)

func TestSplitTopLevelCSVAndSetAssignmentsEscapes(t *testing.T) {
	got := splitTopLevelCSV(`a, 'x,''y', "i,""j", fn(1,2), ${a,b}, c`)
	if len(got) != 6 {
		t.Fatalf("splitTopLevelCSV() len = %d, want 6; got=%v", len(got), got)
	}

	sets := splitSetAssignments(`name='x,''y', note="i,""j", payload=jsonb_build_object('a,b', ${v}), flag=true`)
	if len(sets) != 4 {
		t.Fatalf("splitSetAssignments() len = %d, want 4; got=%v", len(sets), sets)
	}
}

func TestFormatDeleteSQLBranches(t *testing.T) {
	got := formatDeleteSQL("DELETE FROM users RETURNING id")
	want := "DELETE FROM users\nRETURNING id"
	if got != want {
		t.Fatalf("formatDeleteSQL() = %q, want %q", got, want)
	}

	got = formatDeleteSQL("DELETE FROM users")
	if got != "DELETE FROM users" {
		t.Fatalf("formatDeleteSQL(no where) = %q, want %q", got, "DELETE FROM users")
	}
}

func TestFormatWhereClauseWithoutPrefix(t *testing.T) {
	got := formatWhereClause("id = ${id}")
	if got != "id = ${id}" {
		t.Fatalf("formatWhereClause() = %q, want unchanged", got)
	}
}

func TestFormatSetValueBranches(t *testing.T) {
	if got := formatSetValue(SetValue{ExprSQL: "NOW()"}); got != "NOW()" {
		t.Fatalf("formatSetValue(expr) = %q, want NOW()", got)
	}
	if got := formatSetValue(SetValue{IsParam: true, ParamName: "id"}); got != "${id}" {
		t.Fatalf("formatSetValue(param) = %q, want ${id}", got)
	}
	if got := formatSetValue(SetValue{IsLiteral: true, LiteralValue: `"x"`, LiteralKind: token.STRING}); got != "'x'" {
		t.Fatalf("formatSetValue(literal) = %q, want 'x'", got)
	}
	if got := formatSetValue(SetValue{}); got != "" {
		t.Fatalf("formatSetValue(empty) = %q, want empty", got)
	}
}

func TestFormatFuncArgAndResolveColumnNameBranches(t *testing.T) {
	typ := stubType("User")
	pkg := &Package{
		Tables: map[string]*TableBinding{
			getTypeKey(typ): {
				Type:      typ,
				TypeName:  "User",
				TableName: "users",
				Fields: []FieldInfo{
					{GoName: "ID", DBName: "id", Type: types.Typ[types.Int64]},
				},
			},
		},
	}

	path := []FieldPathElement{
		{VarName: "user", Type: typ},
		{FieldName: "ID"},
	}

	if got := resolveColumnName(pkg, nil); got != "" {
		t.Fatalf("resolveColumnName(nil) = %q, want empty", got)
	}
	if got := resolveColumnName(pkg, path); got != "id" {
		t.Fatalf("resolveColumnName(path) = %q, want id", got)
	}

	if got := formatFuncArg(pkg, FuncArg{IsField: true, FieldPath: path}); got != "id" {
		t.Fatalf("formatFuncArg(field) = %q, want id", got)
	}
	if got := formatFuncArg(pkg, FuncArg{IsParam: true, Value: "id"}); got != "${id}" {
		t.Fatalf("formatFuncArg(param) = %q, want ${id}", got)
	}
	if got := formatFuncArg(pkg, FuncArg{IsLiteral: true, Value: `"x"`, Kind: token.STRING}); got != "'x'" {
		t.Fatalf("formatFuncArg(string literal) = %q, want 'x'", got)
	}
	if got := formatFuncArg(pkg, FuncArg{IsLiteral: true, Value: "10", Kind: token.INT}); got != "10" {
		t.Fatalf("formatFuncArg(int literal) = %q, want 10", got)
	}
	if got := formatFuncArg(pkg, FuncArg{}); got != "" {
		t.Fatalf("formatFuncArg(empty) = %q, want empty", got)
	}
}
