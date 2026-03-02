package defgen

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

func TestFormatFilterTreeEdgeBranches(t *testing.T) {
	if got := formatFilterTree(nil); got != "" {
		t.Fatalf("formatFilterTree(nil) = %q, want empty", got)
	}

	andEmpty := &AnalyzedFilter{Kind: AnalyzedFilterAnd}
	if got := formatFilterTree(andEmpty); got != "" {
		t.Fatalf("formatFilterTree(and empty) = %q, want empty", got)
	}

	andSingle := &AnalyzedFilter{
		Kind: AnalyzedFilterAnd,
		Children: []*AnalyzedFilter{
			{Kind: AnalyzedFilterComparison, ColumnName: "id", Operator: "=", Value: "${id}"},
			nil,
		},
	}
	if got := formatFilterTree(andSingle); got != "id = ${id}" {
		t.Fatalf("formatFilterTree(and single) = %q", got)
	}

	orEmpty := &AnalyzedFilter{Kind: AnalyzedFilterOr}
	if got := formatFilterTree(orEmpty); got != "" {
		t.Fatalf("formatFilterTree(or empty) = %q, want empty", got)
	}

	orSingle := &AnalyzedFilter{
		Kind: AnalyzedFilterOr,
		Children: []*AnalyzedFilter{
			{Kind: AnalyzedFilterComparison, ColumnName: "name", Operator: "=", Value: "'x'"},
		},
	}
	if got := formatFilterTree(orSingle); got != "name = 'x'" {
		t.Fatalf("formatFilterTree(or single) = %q", got)
	}

	subqueryWithFuncRight := &AnalyzedFilter{
		Kind:            AnalyzedFilterComparison,
		IsSubquery:      true,
		ForeignKeyCol:   "user_id",
		SubqueryTable:   "users",
		SubqueryIDField: "id",
		SubqueryColumn:  "updated_at",
		Operator:        "=",
		RightIsFunc:     true,
		RightFuncExpr:   "NOW()",
	}
	if got := formatFilterTree(subqueryWithFuncRight); !strings.Contains(got, "NOW()") {
		t.Fatalf("formatFilterTree(subquery func right) = %q", got)
	}
}

func TestGenerateSQLAndDeleteSkipEmptyConditionBranch(t *testing.T) {
	pkg, userType := newSQLTestPackage()
	key := getTypeKey(userType)

	sql, err := GenerateSQL(pkg, &QueryMethod{
		Name: "SkipEmptyCondition",
		ReturnType: ReturnTypeInfo{
			Type: userType,
		},
		Filters: []*FilterExpr{{Kind: FilterAnd}},
	})
	if err != nil || !strings.Contains(sql, "SELECT * FROM users") || strings.Contains(sql, "WHERE") {
		t.Fatalf("GenerateSQL(skip empty cond) = (%q, %v)", sql, err)
	}

	delSQL, err := generateDeleteSQL(pkg, &MutationMethod{
		Name:       "DeleteSkipEmpty",
		Kind:       MethodKindDelete,
		TargetType: key,
		Filters: []*FilterExpr{
			{Kind: FilterAnd},
		},
	})
	if err != nil || delSQL != "DELETE FROM users" {
		t.Fatalf("generateDeleteSQL(skip empty cond) = (%q, %v)", delSQL, err)
	}
}

func TestFormatWithSQLAdditionalBranches(t *testing.T) {
	raw := "WITH  SELECT id FROM users"
	if got := formatWithSQL(raw); got != raw {
		t.Fatalf("formatWithSQL(empty cte) = %q, want raw", got)
	}

	withNonParens := formatWithSQL("WITH c AS SELECT 1 UPDATE users SET name = ${name}")
	if !strings.Contains(withNonParens, "WITH c AS") || !strings.Contains(withNonParens, "SELECT 1 UPDATE users") {
		t.Fatalf("formatWithSQL(non-parens) = %q", withNonParens)
	}

	withTwoCTE := formatWithSQL("WITH a AS (SELECT 1), b AS (SELECT 2) UPDATE users SET name = ${name}")
	if !strings.Contains(withTwoCTE, ",\n") {
		t.Fatalf("formatWithSQL(two ctes) = %q", withTwoCTE)
	}
}

func TestParseTableBindingsExternalStructBranch(t *testing.T) {
	defIdent := ast.NewIdent("def")
	externalType := namedStructType("example.com/ext", "ext", "External", []namedField{
		{name: "ID", typ: types.Typ[types.Int64], tag: `db:"id" primary_key:"true"`},
	})
	externalIdent := ast.NewIdent("External")

	bindCall := &ast.CallExpr{
		Fun: &ast.IndexExpr{
			X: &ast.SelectorExpr{
				X:   defIdent,
				Sel: ast.NewIdent("BindTable"),
			},
			Index: externalIdent,
		},
		Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"externals"`}},
	}
	initCall := makeDefCall(defIdent, "Init", bindCall)

	file := &ast.File{
		Name: ast.NewIdent("store"),
		Decls: []ast.Decl{
			&ast.GenDecl{
				Tok: token.VAR,
				Specs: []ast.Spec{
					&ast.ValueSpec{
						Names:  []*ast.Ident{ast.NewIdent("_")},
						Values: []ast.Expr{initCall},
					},
				},
			},
		},
	}

	pkg := &Package{
		Syntax: []*ast.File{file},
		TypesInfo: &types.Info{
			Uses: map[*ast.Ident]types.Object{
				defIdent:      types.NewPkgName(token.NoPos, nil, "def", types.NewPackage(defPkgPath, "def")),
				externalIdent: externalType.Obj(),
			},
		},
		Tables: map[string]*TableBinding{},
	}
	structs := map[string]*structInfo{}

	if err := parseTableBindings(pkg, structs); err != nil {
		t.Fatalf("parseTableBindings() error = %v", err)
	}
	key := getTypeKey(externalType)
	if _, ok := pkg.Tables[key]; !ok {
		t.Fatalf("parseTableBindings() missing table for key %q", key)
	}
	if _, ok := structs[key]; !ok {
		t.Fatalf("parseTableBindings() should cache external struct info")
	}
}

func TestParseFieldPathForeignRefCacheMissBranch(t *testing.T) {
	refType := namedStructType("example.com/demo", "demo", "Ref", []namedField{
		{name: "Name", typ: types.Typ[types.String], tag: `db:"name"`},
	})
	sourceType := namedStructType("example.com/demo", "demo", "Source", []namedField{
		{name: "ID", typ: types.Typ[types.Int64], tag: `db:"id" primary_key:"true"`},
		{name: "RefID", typ: types.Typ[types.Int64], tag: `db:"ref_id"`},
		{name: "Ref", typ: types.NewPointer(refType), tag: `db:"-" foreign_key:"ref_id"`},
	})

	sourceIdent := ast.NewIdent("src")
	pkg := &Package{
		TypesInfo: &types.Info{
			Uses: map[*ast.Ident]types.Object{
				sourceIdent: types.NewVar(token.NoPos, nil, "src", sourceType),
			},
		},
	}

	structs := map[string]*structInfo{
		getTypeKey(sourceType): getStructInfoFromType(sourceType),
	}

	sel := &ast.SelectorExpr{
		X: &ast.SelectorExpr{
			X:   sourceIdent,
			Sel: ast.NewIdent("Ref"),
		},
		Sel: ast.NewIdent("Name"),
	}
	path, err := parseFieldPath(pkg, sel, structs)
	if err != nil {
		t.Fatalf("parseFieldPath() error = %v", err)
	}
	if len(path) != 3 || !path[1].IsForeignKey {
		t.Fatalf("parseFieldPath() path = %+v, want foreign key traversal", path)
	}
	if _, ok := structs[getTypeKey(refType)]; !ok {
		t.Fatalf("parseFieldPath() should cache ref struct on miss")
	}
}
