package defgen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

func TestParseBindTableCallAndDefCallBranches(t *testing.T) {
	defIdent := ast.NewIdent("def")
	otherIdent := ast.NewIdent("other")
	pkg := &Package{
		TypesInfo: &types.Info{
			Uses: map[*ast.Ident]types.Object{
				defIdent:   types.NewPkgName(token.NoPos, nil, "def", types.NewPackage(defPkgPath, "def")),
				otherIdent: types.NewPkgName(token.NoPos, nil, "other", types.NewPackage("example.com/other", "other")),
			},
		},
	}

	call := &ast.CallExpr{
		Fun: &ast.IndexExpr{
			X: &ast.SelectorExpr{
				X:   defIdent,
				Sel: ast.NewIdent("BindTable"),
			},
			Index: ast.NewIdent("User"),
		},
		Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"users"`}},
	}

	typeName, tableName, _, ok := parseBindTableCall(pkg, call)
	if !ok || typeName != "User" || tableName != "users" {
		t.Fatalf("parseBindTableCall() = (%q,%q,%v), want User/users/true", typeName, tableName, ok)
	}

	selectorTypeCall := &ast.CallExpr{
		Fun: &ast.IndexExpr{
			X: &ast.SelectorExpr{
				X:   defIdent,
				Sel: ast.NewIdent("BindTable"),
			},
			Index: &ast.SelectorExpr{
				X:   ast.NewIdent("entity"),
				Sel: ast.NewIdent("Project"),
			},
		},
		Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"projects"`}},
	}
	typeName, tableName, _, ok = parseBindTableCall(pkg, selectorTypeCall)
	if !ok || typeName != "Project" || tableName != "projects" {
		t.Fatalf("parseBindTableCall(selector type) = (%q,%q,%v), want Project/projects/true", typeName, tableName, ok)
	}

	badCalls := []*ast.CallExpr{
		{Fun: ast.NewIdent("x")},
		{Fun: &ast.IndexExpr{X: ast.NewIdent("x"), Index: ast.NewIdent("T")}},
		{Fun: &ast.IndexExpr{X: &ast.SelectorExpr{X: defIdent, Sel: ast.NewIdent("Query")}, Index: ast.NewIdent("T")}},
		{Fun: &ast.IndexExpr{X: &ast.SelectorExpr{X: otherIdent, Sel: ast.NewIdent("BindTable")}, Index: ast.NewIdent("T")}},
		{Fun: &ast.IndexExpr{X: &ast.SelectorExpr{X: defIdent, Sel: ast.NewIdent("BindTable")}, Index: &ast.BasicLit{Kind: token.INT, Value: "1"}}, Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"x"`}}},
		{Fun: &ast.IndexExpr{X: &ast.SelectorExpr{X: defIdent, Sel: ast.NewIdent("BindTable")}, Index: ast.NewIdent("T")}},
		{Fun: &ast.IndexExpr{X: &ast.SelectorExpr{X: defIdent, Sel: ast.NewIdent("BindTable")}, Index: ast.NewIdent("T")}, Args: []ast.Expr{ast.NewIdent("x")}},
	}
	for i, c := range badCalls {
		if _, _, _, ok := parseBindTableCall(pkg, c); ok {
			t.Fatalf("parseBindTableCall(bad[%d]) expected false", i)
		}
	}

	if !isDefCall(pkg, makeDefCall(defIdent, "Query"), "Query") {
		t.Fatalf("isDefCall() should detect def.Query")
	}
	if isDefCall(pkg, makeDefCall(otherIdent, "Query"), "Query") {
		t.Fatalf("isDefCall() should reject non-def package")
	}

	if _, ok := isGenericDefCall(pkg, makeDefGenericCall(defIdent, "Count", ast.NewIdent("int64"), ast.NewIdent("x"))); !ok {
		t.Fatalf("isGenericDefCall() should detect generic def call")
	}
	if _, ok := isGenericDefCall(pkg, &ast.CallExpr{Fun: ast.NewIdent("x")}); ok {
		t.Fatalf("isGenericDefCall() should reject non-index call")
	}
}

func TestSchemaAndRelationBranchHelpers(t *testing.T) {
	named := namedStructType("example.com/demo", "demo", "User", []namedField{
		{name: "ID", typ: types.Typ[types.Int64], tag: `db:"id" primary_key:"true"`},
	})

	if got := getTypeName(named); got != "User" {
		t.Fatalf("getTypeName(named) = %q, want User", got)
	}
	if got := getTypeName(types.NewPointer(named)); got != "User" {
		t.Fatalf("getTypeName(pointer) = %q, want User", got)
	}
	if got := getTypeName(types.NewSlice(named)); got != "User" {
		t.Fatalf("getTypeName(slice) = %q, want User", got)
	}
	if got := getTypeName(types.Typ[types.Int]); got != "" {
		t.Fatalf("getTypeName(int) = %q, want empty", got)
	}

	if _, err := lookupTableByType(&Package{Tables: map[string]*TableBinding{}}, types.Typ[types.Int]); err == nil {
		t.Fatalf("lookupTableByType(int) expected error")
	}
	if _, err := lookupTableByTargetType(&Package{Tables: map[string]*TableBinding{}}, ""); err == nil {
		t.Fatalf("lookupTableByTargetType(empty) expected error")
	}

	pkg := &Package{Tables: map[string]*TableBinding{}}
	addTableBindingFromNamed(pkg, named, "users")
	if _, err := lookupTableByType(pkg, named); err != nil {
		t.Fatalf("lookupTableByType(named) error = %v", err)
	}

	refType := namedStructType("example.com/demo", "demo", "Ref", []namedField{
		{name: "Name", typ: types.Typ[types.String], tag: `db:"name"`},
	})
	sourceType := namedStructType("example.com/demo", "demo", "Source", []namedField{
		{name: "ID", typ: types.Typ[types.Int64], tag: `db:"id" primary_key:"true"`},
		{name: "RefID", typ: types.Typ[types.Int64], tag: `db:"ref_id"`},
		{name: "Ref", typ: types.NewPointer(refType), tag: `db:"-" foreign_key:"ref_id"`},
	})
	pkg2 := &Package{Tables: map[string]*TableBinding{}}
	addTableBindingFromNamed(pkg2, sourceType, "sources")
	addTableBindingFromNamed(pkg2, refType, "refs")
	pkg2.Tables[getTypeKey(refType)].PrimaryKey = nil

	fk := getStructInfoFromType(sourceType).foreignKeys[0]
	if got := analyzeBelongsTo(pkg2, pkg2.Tables[getTypeKey(sourceType)], fk); got != nil {
		t.Fatalf("analyzeBelongsTo() with missing pk should return nil")
	}
}

func TestParseInterfaceDefsUnnamedParamBranch(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "iface.go", `package demo
type Store interface {
	Do(context.Context, int) error
}
`, 0)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	pkg := &Package{
		Fset:       fset,
		Syntax:     []*ast.File{file},
		TypesInfo:  &types.Info{Types: map[ast.Expr]types.TypeAndValue{}},
		Interfaces: map[string]*InterfaceInfo{},
	}

	parseInterfaceDefs(pkg)
	iface := pkg.Interfaces["Store"]
	if iface == nil || len(iface.Methods) != 1 {
		t.Fatalf("parseInterfaceDefs() missing interface methods: %+v", iface)
	}
	if len(iface.Methods[0].Params) != 2 || iface.Methods[0].Params[0].Name != "" {
		t.Fatalf("parseInterfaceDefs() unnamed params not captured: %+v", iface.Methods[0].Params)
	}
}

func TestSQLBranchHelpers(t *testing.T) {
	if got := formatFilterTree(&AnalyzedFilter{
		Kind:            AnalyzedFilterIn,
		IsSubquery:      true,
		ForeignKeyCol:   "user_id",
		SubqueryTable:   "users",
		SubqueryIDField: "id",
		SubqueryColumn:  "id",
		Value:           "${ids}",
	}); !strings.Contains(got, "IN (SELECT") {
		t.Fatalf("formatFilterTree(subquery in) = %q", got)
	}

	if got := FormatSQL("TRUNCATE users"); got != "TRUNCATE users" {
		t.Fatalf("FormatSQL(default) = %q, want unchanged", got)
	}

	withNoMain := formatWithSQL("WITH cte")
	if withNoMain != "WITH cte" {
		t.Fatalf("formatWithSQL(no main) = %q", withNoMain)
	}

	withFallback := formatWithSQL("WITH raw UPDATE users SET name = ${name}")
	if !strings.Contains(withFallback, "WITH raw") || !strings.Contains(withFallback, "UPDATE users") {
		t.Fatalf("formatWithSQL(fallback) = %q", withFallback)
	}

	withQuoted := formatWithSQL("WITH c AS (SELECT 'a''b') UPDATE users SET name = ${name}")
	if !strings.Contains(withQuoted, "WITH c AS (") {
		t.Fatalf("formatWithSQL(quoted) = %q", withQuoted)
	}

	withBadAS := formatWithSQL("WITH c AS SELECT 1 UPDATE users SET name = ${name}")
	if !strings.Contains(withBadAS, "WITH c AS") || !strings.Contains(withBadAS, "SELECT 1 UPDATE users") {
		t.Fatalf("formatWithSQL(bad as) = %q", withBadAS)
	}

	if q, v := parseFromSourceQualifier("schema.table AS t"); q != "schema.table" || v != "t" {
		t.Fatalf("parseFromSourceQualifier() = (%q,%q), want (schema.table,t)", q, v)
	}
}

func TestGenerateSQLAndMutationBranchErrors(t *testing.T) {
	_, err := GenerateSQL(&Package{Tables: map[string]*TableBinding{}}, &QueryMethod{
		Name: "Q",
		ReturnType: ReturnTypeInfo{
			Type: types.Typ[types.Int],
		},
	})
	if err == nil {
		t.Fatalf("GenerateSQL() expected table lookup error")
	}

	_, err = GenerateMutationSQL(&Package{}, &MutationMethod{Kind: MethodKind(99)})
	if err == nil || !strings.Contains(err.Error(), "unknown mutation kind") {
		t.Fatalf("GenerateMutationSQL() error = %v, want unknown kind", err)
	}

	conds, err := buildFilterConditions(&Package{}, nil, nil)
	if err != nil || len(conds) != 0 {
		t.Fatalf("buildFilterConditions(nil) = (%v, %v), want empty nil", conds, err)
	}
}
