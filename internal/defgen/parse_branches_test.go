package defgen

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

type parseTestContext struct {
	pkg       *Package
	structs   map[string]*structInfo
	defIdent  *ast.Ident
	pgIdent   *ast.Ident
	userIdent *ast.Ident
	taskIdent *ast.Ident
	userType  *types.Named
	taskType  *types.Named
}

func newParseTestContext() parseTestContext {
	userType := namedStructType("example.com/demo", "demo", "User", []namedField{
		{name: "ID", typ: types.Typ[types.Int64], tag: `db:"id" primary_key:"true"`},
		{name: "Name", typ: types.Typ[types.String], tag: `db:"name"`},
		{name: "DeletedAt", typ: types.Typ[types.String], tag: `db:"deleted_at"`},
	})
	taskType := namedStructType("example.com/demo", "demo", "Task", []namedField{
		{name: "ID", typ: types.Typ[types.Int64], tag: `db:"id" primary_key:"true"`},
		{name: "UserID", typ: types.Typ[types.Int64], tag: `db:"user_id"`},
		{name: "Title", typ: types.Typ[types.String], tag: `db:"title"`},
		{name: "User", typ: types.NewPointer(userType), tag: `db:"-" foreign_key:"user_id"`},
	})

	defIdent := ast.NewIdent("def")
	pgIdent := ast.NewIdent("postgres")
	userIdent := ast.NewIdent("user")
	taskIdent := ast.NewIdent("task")

	pkg := &Package{
		TypesInfo: &types.Info{
			Uses: map[*ast.Ident]types.Object{
				defIdent:  types.NewPkgName(token.NoPos, nil, "def", types.NewPackage(defPkgPath, "def")),
				pgIdent:   types.NewPkgName(token.NoPos, nil, "postgres", types.NewPackage(postgresPkgPath, "postgres")),
				userIdent: types.NewVar(token.NoPos, nil, "user", userType),
				taskIdent: types.NewVar(token.NoPos, nil, "task", taskType),
			},
		},
		Tables: map[string]*TableBinding{},
	}

	addTableBindingFromNamed(pkg, userType, "users")
	addTableBindingFromNamed(pkg, taskType, "tasks")

	structs := map[string]*structInfo{
		getTypeKey(userType): getStructInfoFromType(userType),
		getTypeKey(taskType): getStructInfoFromType(taskType),
	}

	return parseTestContext{
		pkg:       pkg,
		structs:   structs,
		defIdent:  defIdent,
		pgIdent:   pgIdent,
		userIdent: userIdent,
		taskIdent: taskIdent,
		userType:  userType,
		taskType:  taskType,
	}
}

type namedField struct {
	name string
	typ  types.Type
	tag  string
}

func namedStructType(pkgPath, pkgName, typeName string, fields []namedField) *types.Named {
	p := types.NewPackage(pkgPath, pkgName)
	var vars []*types.Var
	var tags []string
	for _, f := range fields {
		vars = append(vars, types.NewVar(token.NoPos, p, f.name, f.typ))
		tags = append(tags, f.tag)
	}
	obj := types.NewTypeName(token.NoPos, p, typeName, nil)
	return types.NewNamed(obj, types.NewStruct(vars, tags), nil)
}

func addTableBindingFromNamed(pkg *Package, t *types.Named, tableName string) {
	si := getStructInfoFromType(t)
	if si == nil {
		return
	}
	binding := &TableBinding{
		Type:        t,
		TypeName:    t.Obj().Name(),
		TableName:   tableName,
		Fields:      si.fields,
		ForeignKeys: si.foreignKeys,
	}
	for i := range binding.Fields {
		if binding.Fields[i].IsPrimaryKey {
			binding.PrimaryKey = &binding.Fields[i]
			break
		}
	}
	pkg.Tables[getTypeKey(t)] = binding
}

func makeDefCall(defIdent *ast.Ident, name string, args ...ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   defIdent,
			Sel: ast.NewIdent(name),
		},
		Args: args,
	}
}

func makePostgresCall(pgIdent *ast.Ident, name string, args ...ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   pgIdent,
			Sel: ast.NewIdent(name),
		},
		Args: args,
	}
}

func makeDefGenericCall(defIdent *ast.Ident, name string, index ast.Expr, args ...ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun: &ast.IndexExpr{
			X: &ast.SelectorExpr{
				X:   defIdent,
				Sel: ast.NewIdent(name),
			},
			Index: index,
		},
		Args: args,
	}
}

func TestParsePaginationExprBranches(t *testing.T) {
	params := []ParamInfo{{Name: "limit", Type: types.Typ[types.Int]}}

	tests := []struct {
		name    string
		call    *ast.CallExpr
		wantErr string
	}{
		{
			name:    "invalid arg count",
			call:    &ast.CallExpr{Args: []ast.Expr{ast.NewIdent("a"), ast.NewIdent("b")}},
			wantErr: "expected exactly 1 argument",
		},
		{
			name:    "invalid integer literal",
			call:    &ast.CallExpr{Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "9x"}}},
			wantErr: "invalid integer literal",
		},
		{
			name:    "unknown identifier",
			call:    &ast.CallExpr{Args: []ast.Expr{ast.NewIdent("missing")}},
			wantErr: "unknown identifier",
		},
		{
			name:    "unsupported type",
			call:    &ast.CallExpr{Args: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent("u"), Sel: ast.NewIdent("ID")}}},
			wantErr: "integer literal or parameter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parsePaginationExpr(tt.call, params)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("parsePaginationExpr() error = %v, want contains %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseFuncArgBranches(t *testing.T) {
	ctx := newParseTestContext()
	params := []ParamInfo{{Name: "name", Type: types.Typ[types.String]}}

	selector := &ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("Name")}
	got, err := parseFuncArg(ctx.pkg, selector, ctx.structs, params)
	if err != nil {
		t.Fatalf("parseFuncArg(selector) error = %v", err)
	}
	if !got.IsField || len(got.FieldPath) < 2 {
		t.Fatalf("parseFuncArg(selector) = %+v, want field path", got)
	}

	got, err = parseFuncArg(ctx.pkg, ast.NewIdent("name"), ctx.structs, params)
	if err != nil {
		t.Fatalf("parseFuncArg(param) error = %v", err)
	}
	if !got.IsParam || got.Value != "name" {
		t.Fatalf("parseFuncArg(param) = %+v, want parameter reference", got)
	}

	_, err = parseFuncArg(ctx.pkg, ast.NewIdent("missing"), ctx.structs, params)
	if err == nil || !strings.Contains(err.Error(), "unknown identifier") {
		t.Fatalf("parseFuncArg(unknown) error = %v, want unknown identifier", err)
	}

	_, err = parseFuncArg(ctx.pkg, &ast.CallExpr{Fun: ast.NewIdent("now")}, ctx.structs, params)
	if err == nil || !strings.Contains(err.Error(), "unsupported function argument type") {
		t.Fatalf("parseFuncArg(unsupported) error = %v, want unsupported type", err)
	}
}

func TestParseFieldPathFromExprError(t *testing.T) {
	ctx := newParseTestContext()
	_, err := parseFieldPathFromExpr(ctx.pkg, ast.NewIdent("x"), ctx.structs)
	if err == nil || !strings.Contains(err.Error(), "expected selector expression") {
		t.Fatalf("parseFieldPathFromExpr() error = %v, want selector error", err)
	}
}

func TestParseFilterOperandBranches(t *testing.T) {
	ctx := newParseTestContext()
	params := []ParamInfo{{Name: "name", Type: types.Typ[types.String]}}

	tests := []struct {
		name    string
		expr    ast.Expr
		wantErr string
	}{
		{
			name: "literal",
			expr: &ast.BasicLit{Kind: token.INT, Value: "1"},
		},
		{
			name: "postgres now",
			expr: makePostgresCall(ctx.pgIdent, "Now"),
		},
		{
			name:    "generic count with unknown arg",
			expr:    makeDefGenericCall(ctx.defIdent, "Count", ast.NewIdent("int64"), ast.NewIdent("missing")),
			wantErr: "unknown identifier",
		},
		{
			name:    "generic func missing function name",
			expr:    makeDefGenericCall(ctx.defIdent, "Func", ast.NewIdent("string")),
			wantErr: "requires at least 1 argument",
		},
		{
			name:    "generic func first arg not string",
			expr:    makeDefGenericCall(ctx.defIdent, "Func", ast.NewIdent("string"), ast.NewIdent("name")),
			wantErr: "first argument must be a string literal",
		},
		{
			name:    "unsupported def generic function",
			expr:    makeDefGenericCall(ctx.defIdent, "BindTable", ast.NewIdent("any"), ast.NewIdent("name")),
			wantErr: "unsupported def function in filter",
		},
		{
			name:    "unsupported call expression",
			expr:    &ast.CallExpr{Fun: ast.NewIdent("custom")},
			wantErr: "unsupported call expression in filter operand",
		},
		{
			name:    "unsupported expression type",
			expr:    &ast.BinaryExpr{X: ast.NewIdent("a"), Op: token.ADD, Y: ast.NewIdent("b")},
			wantErr: "unsupported expression type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFilterOperand(ctx.pkg, tt.expr, ctx.structs, params, false)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseFilterOperand() error = %v, want contains %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFilterOperand() error = %v", err)
			}
			if !got.IsLiteral && !got.IsFunc {
				t.Fatalf("parseFilterOperand() = %+v, want literal or function", got)
			}
		})
	}
}

func TestParseWithAndFromSourceBranches(t *testing.T) {
	ctx := newParseTestContext()
	params := []ParamInfo{{Name: "offset", Type: types.Typ[types.Int]}}

	_, err := parseWithClause(ctx.pkg, makeDefCall(ctx.defIdent, "With"), ctx.structs, params)
	if err == nil || !strings.Contains(err.Error(), "requires a name and at least one clause") {
		t.Fatalf("parseWithClause(no args) error = %v", err)
	}

	badNameCall := makeDefCall(ctx.defIdent, "With", &ast.BasicLit{Kind: token.INT, Value: "1"}, makeDefCall(ctx.defIdent, "From", ctx.taskIdent))
	_, err = parseWithClause(ctx.pkg, badNameCall, ctx.structs, params)
	if err == nil || !strings.Contains(err.Error(), "first argument must be a string literal") {
		t.Fatalf("parseWithClause(bad name) error = %v", err)
	}

	unsupportedArgCall := makeDefCall(ctx.defIdent, "With",
		&ast.BasicLit{Kind: token.STRING, Value: `"due"`},
		ast.NewIdent("oops"),
	)
	_, err = parseWithClause(ctx.pkg, unsupportedArgCall, ctx.structs, params)
	if err == nil || !strings.Contains(err.Error(), "unsupported argument in def.With") {
		t.Fatalf("parseWithClause(unsupported arg) error = %v", err)
	}

	missingFromCall := makeDefCall(ctx.defIdent, "With",
		&ast.BasicLit{Kind: token.STRING, Value: `"due"`},
		makeDefCall(ctx.defIdent, "Offset", ast.NewIdent("offset")),
	)
	_, err = parseWithClause(ctx.pkg, missingFromCall, ctx.structs, params)
	if err == nil || !strings.Contains(err.Error(), "requires def.From(tableVar)") {
		t.Fatalf("parseWithClause(missing from) error = %v", err)
	}

	validCall := makeDefCall(ctx.defIdent, "With",
		&ast.BasicLit{Kind: token.STRING, Value: `"due"`},
		makeDefCall(ctx.defIdent, "From", ctx.taskIdent),
		makeDefCall(ctx.defIdent, "OrderBy", makeDefCall(ctx.defIdent, "Desc", &ast.SelectorExpr{X: ctx.taskIdent, Sel: ast.NewIdent("ID")})),
		makeDefCall(ctx.defIdent, "Offset", ast.NewIdent("offset")),
	)
	withClause, err := parseWithClause(ctx.pkg, validCall, ctx.structs, params)
	if err != nil {
		t.Fatalf("parseWithClause(valid) error = %v", err)
	}
	if withClause.Offset == nil || !withClause.Offset.IsParam || withClause.Offset.ParamName != "offset" {
		t.Fatalf("parseWithClause(valid).Offset = %+v, want param offset", withClause.Offset)
	}
	if len(withClause.OrderBy) != 1 || !withClause.OrderBy[0].Desc {
		t.Fatalf("parseWithClause(valid).OrderBy = %+v, want one DESC expression", withClause.OrderBy)
	}

	_, err = parseWithFromSource(ctx.pkg, &ast.CallExpr{Args: []ast.Expr{}})
	if err == nil || !strings.Contains(err.Error(), "requires exactly 1 argument") {
		t.Fatalf("parseWithFromSource(arg count) error = %v", err)
	}

	_, err = parseWithFromSource(&Package{TypesInfo: &types.Info{}, Tables: map[string]*TableBinding{}}, &ast.CallExpr{Args: []ast.Expr{ast.NewIdent("x")}})
	if err == nil || !strings.Contains(err.Error(), "unable to infer source type") {
		t.Fatalf("parseWithFromSource(type infer) error = %v", err)
	}

	intIdent := ast.NewIdent("n")
	pkgNoBinding := &Package{
		TypesInfo: &types.Info{
			Uses: map[*ast.Ident]types.Object{
				intIdent: types.NewVar(token.NoPos, nil, "n", types.Typ[types.Int]),
			},
		},
		Tables: map[string]*TableBinding{},
	}
	_, err = parseWithFromSource(pkgNoBinding, &ast.CallExpr{Args: []ast.Expr{intIdent}})
	if err == nil || !strings.Contains(err.Error(), "not a supported bound table type") {
		t.Fatalf("parseWithFromSource(non-bound type) error = %v", err)
	}

	user2Ident := ast.NewIdent("user2")
	pkgMissingBinding := &Package{
		TypesInfo: &types.Info{
			Uses: map[*ast.Ident]types.Object{
				user2Ident: types.NewVar(token.NoPos, nil, "user2", ctx.userType),
			},
		},
		Tables: map[string]*TableBinding{},
	}
	_, err = parseWithFromSource(pkgMissingBinding, &ast.CallExpr{Args: []ast.Expr{user2Ident}})
	if err == nil || !strings.Contains(err.Error(), "table binding not found") {
		t.Fatalf("parseWithFromSource(missing binding) error = %v", err)
	}
}

func TestParseUpdateFromSourceBranches(t *testing.T) {
	tests := []struct {
		name    string
		call    *ast.CallExpr
		wantErr string
	}{
		{
			name:    "invalid arg count",
			call:    &ast.CallExpr{Args: []ast.Expr{}},
			wantErr: "requires exactly 1 argument",
		},
		{
			name:    "non-string arg",
			call:    &ast.CallExpr{Args: []ast.Expr{ast.NewIdent("x")}},
			wantErr: "requires a source name string literal",
		},
		{
			name:    "invalid string literal",
			call:    &ast.CallExpr{Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: "\"bad"}}},
			wantErr: "invalid def.From source",
		},
		{
			name:    "empty source name",
			call:    &ast.CallExpr{Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"   "`}}},
			wantErr: "must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseUpdateFromSource(tt.call)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("parseUpdateFromSource() error = %v, want contains %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseOrderByAndSetBranches(t *testing.T) {
	ctx := newParseTestContext()

	_, err := parseOrderByExprs(ctx.pkg, makeDefCall(ctx.defIdent, "OrderBy"), ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "requires at least 1 argument") {
		t.Fatalf("parseOrderByExprs() error = %v", err)
	}

	_, err = parseOrderByExpr(ctx.pkg, makeDefCall(ctx.defIdent, "Asc"), ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "requires exactly 1 argument") {
		t.Fatalf("parseOrderByExpr(bad asc) error = %v", err)
	}

	_, err = parseOrderByExpr(ctx.pkg, ast.NewIdent("bad"), ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to parse ORDER BY expression") {
		t.Fatalf("parseOrderByExpr(bad expr) error = %v", err)
	}

	_, err = parseSetExpr(ctx.pkg, makeDefCall(ctx.defIdent, "Set", ast.NewIdent("x")), ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "requires exactly 2 arguments") {
		t.Fatalf("parseSetExpr(arg count) error = %v", err)
	}

	_, err = parseSetExpr(ctx.pkg, makeDefCall(ctx.defIdent, "Set", ast.NewIdent("x"), ast.NewIdent("y")), ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "first argument must be a field selector") {
		t.Fatalf("parseSetExpr(non selector) error = %v", err)
	}
}

func TestParseSetValueAndBuildExprBranches(t *testing.T) {
	ctx := newParseTestContext()
	params := []ParamInfo{{Name: "delta", Type: types.Typ[types.Int]}}

	got, err := parseSetValue(ctx.pkg, ast.NewIdent("nil"), ctx.structs, params)
	if err != nil || got.ExprSQL != "NULL" {
		t.Fatalf("parseSetValue(nil) = %+v, err=%v", got, err)
	}

	got, err = parseSetValue(ctx.pkg, ast.NewIdent("true"), ctx.structs, params)
	if err != nil || got.ExprSQL != "TRUE" {
		t.Fatalf("parseSetValue(true) = %+v, err=%v", got, err)
	}

	_, err = parseSetValue(ctx.pkg, ast.NewIdent("missing"), ctx.structs, params)
	if err == nil || !strings.Contains(err.Error(), "unknown identifier") {
		t.Fatalf("parseSetValue(unknown) error = %v", err)
	}

	got, err = parseSetValue(ctx.pkg, &ast.BasicLit{Kind: token.STRING, Value: `"x"`}, ctx.structs, params)
	if err != nil || !got.IsLiteral {
		t.Fatalf("parseSetValue(literal) = %+v, err=%v", got, err)
	}

	_, err = buildSetValueExprSQL(ctx.pkg, &ast.UnaryExpr{Op: token.NOT, X: ast.NewIdent("delta")}, ctx.structs, params)
	if err == nil || !strings.Contains(err.Error(), "unsupported unary operator") {
		t.Fatalf("buildSetValueExprSQL(unary) error = %v", err)
	}

	_, err = buildSetValueExprSQL(ctx.pkg, &ast.BinaryExpr{X: ast.NewIdent("delta"), Op: token.AND, Y: ast.NewIdent("delta")}, ctx.structs, params)
	if err == nil || !strings.Contains(err.Error(), "unsupported binary operator") {
		t.Fatalf("buildSetValueExprSQL(binary) error = %v", err)
	}

	_, err = buildSetValueExprSQL(ctx.pkg, &ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("Missing")}, ctx.structs, params)
	if err == nil || !strings.Contains(err.Error(), "failed to resolve column name") {
		t.Fatalf("buildSetValueExprSQL(selector missing) error = %v", err)
	}

	_, err = buildSetValueExprSQL(ctx.pkg, makePostgresCall(ctx.pgIdent, "Interval"), ctx.structs, params)
	if err == nil || !strings.Contains(err.Error(), "requires exactly 1 argument") {
		t.Fatalf("buildSetValueExprSQL(interval no arg) error = %v", err)
	}

	_, err = buildSetValueExprSQL(ctx.pkg, makePostgresCall(ctx.pgIdent, "Interval", &ast.BasicLit{Kind: token.INT, Value: "1"}), ctx.structs, params)
	if err == nil || !strings.Contains(err.Error(), "must be a string literal") {
		t.Fatalf("buildSetValueExprSQL(interval non-string) error = %v", err)
	}

	_, err = buildSetValueExprSQL(ctx.pkg, makePostgresCall(ctx.pgIdent, "Excluded"), ctx.structs, params)
	if err == nil || !strings.Contains(err.Error(), "requires exactly 1 argument") {
		t.Fatalf("buildSetValueExprSQL(excluded no arg) error = %v", err)
	}

	_, err = buildSetValueExprSQL(ctx.pkg, makePostgresCall(ctx.pgIdent, "Excluded", ast.NewIdent("x")), ctx.structs, params)
	if err == nil || !strings.Contains(err.Error(), "must be a field expression") {
		t.Fatalf("buildSetValueExprSQL(excluded non-selector) error = %v", err)
	}

	_, err = buildSetValueExprSQL(ctx.pkg, makePostgresCall(ctx.pgIdent, "Excluded", &ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("Missing")}), ctx.structs, params)
	if err == nil || !strings.Contains(err.Error(), "failed to resolve column name") {
		t.Fatalf("buildSetValueExprSQL(excluded missing col) error = %v", err)
	}

	_, err = buildSetValueExprSQL(ctx.pkg, &ast.CallExpr{Fun: &ast.CallExpr{}}, ctx.structs, params)
	if err == nil || !strings.Contains(err.Error(), "unsupported function expression in Set value") {
		t.Fatalf("buildSetValueExprSQL(bad call fun) error = %v", err)
	}

	_, err = buildSetDefFuncSQL(ctx.pkg, makeDefGenericCall(ctx.defIdent, "Func", ast.NewIdent("any")), ctx.structs, params)
	if err == nil || !strings.Contains(err.Error(), "requires at least 1 argument") {
		t.Fatalf("buildSetDefFuncSQL(no args) error = %v", err)
	}
}

func TestOnConflictAndMutationReturnTypeBranches(t *testing.T) {
	ctx := newParseTestContext()

	// Build postgres.OnConflict(task.ID).DoUpdate() AST.
	inner := makePostgresCall(ctx.pgIdent, "OnConflict", &ast.SelectorExpr{X: ctx.taskIdent, Sel: ast.NewIdent("ID")})
	outer := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   inner,
			Sel: ast.NewIdent("DoUpdate"),
		},
	}

	parsedInner, action, ok := isPostgresOnConflictDoCall(ctx.pkg, outer)
	if !ok || action != "update" || parsedInner == nil {
		t.Fatalf("isPostgresOnConflictDoCall() = (%v, %q, %v), want update true", parsedInner, action, ok)
	}

	_, _, err := parseOnConflictExpr(ctx.pkg, outer, inner, "update", ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "requires at least 1 def.Set argument") {
		t.Fatalf("parseOnConflictExpr(do update no set) error = %v", err)
	}

	noColsInner := makePostgresCall(ctx.pgIdent, "OnConflict")
	_, _, err = parseOnConflictExpr(ctx.pkg, outer, noColsInner, "nothing", ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "requires at least 1 column argument") {
		t.Fatalf("parseOnConflictExpr(no columns) error = %v", err)
	}

	badColsInner := makePostgresCall(ctx.pgIdent, "OnConflict", ast.NewIdent("bad"))
	_, _, err = parseOnConflictExpr(ctx.pkg, outer, badColsInner, "nothing", ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to parse OnConflict column") {
		t.Fatalf("parseOnConflictExpr(bad column) error = %v", err)
	}

	mt := analyzeMutationReturnType(types.Typ[types.Int64])
	if mt == nil || !mt.IsScalar || mt.StructName != "int64" {
		t.Fatalf("analyzeMutationReturnType(int64) = %+v, want scalar int64", mt)
	}
}

func TestParseCreateDeleteAndMethodParsingBranches(t *testing.T) {
	ctx := newParseTestContext()

	createCall := makeDefCall(ctx.defIdent, "Create",
		ast.NewIdent("u"),
		&ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   makePostgresCall(ctx.pgIdent, "OnConflict", &ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("ID")}),
				Sel: ast.NewIdent("DoUpdate"),
			},
			Args: []ast.Expr{
				makeDefCall(ctx.defIdent, "Set", &ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("Name")}, &ast.BasicLit{Kind: token.STRING, Value: `"x"`}),
			},
		},
	)

	createResult, err := parseCreateArgs(ctx.pkg, createCall, ctx.structs, []ParamInfo{
		{Name: "u", Type: types.NewPointer(ctx.userType)},
	})
	if err != nil {
		t.Fatalf("parseCreateArgs(entity+on conflict update) error = %v", err)
	}
	if createResult.ConflictAction != "update" || len(createResult.ConflictColumns) == 0 || len(createResult.ConflictSets) == 0 {
		t.Fatalf("parseCreateArgs() conflict fields not parsed: %+v", createResult)
	}

	deleteCall := makeDefCall(ctx.defIdent, "Delete",
		makeDefCall(ctx.defIdent, "Filter", &ast.BinaryExpr{
			X:  &ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("ID")},
			Op: token.EQL,
			Y:  ast.NewIdent("id"),
		}),
		makePostgresCall(ctx.pgIdent, "Returning", &ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("ID")}),
	)
	targetType, filters, returningCols, err := parseDeleteArgs(ctx.pkg, deleteCall, ctx.structs, []ParamInfo{
		{Name: "id", Type: types.Typ[types.Int64]},
	})
	if err != nil {
		t.Fatalf("parseDeleteArgs(returning) error = %v", err)
	}
	if targetType == "" || len(filters) != 1 || len(returningCols) != 1 {
		t.Fatalf("parseDeleteArgs(returning) = (%q, %d, %d), want target+1 filter+1 returning", targetType, len(filters), len(returningCols))
	}

	defIdent := ast.NewIdent("def")
	queryCall := makeDefCall(defIdent, "Query")
	qFn := &ast.FuncDecl{
		Name: ast.NewIdent("Find"),
		Recv: &ast.FieldList{List: []*ast.Field{{Type: &ast.StarExpr{X: ast.NewIdent("repo")}}}},
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					{Type: ast.NewIdent("int")},
				},
			},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{Type: ast.NewIdent("any")},
				},
			},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: queryCall}}},
	}
	pkgForMethods := &Package{
		TypesInfo: &types.Info{
			Uses: map[*ast.Ident]types.Object{
				defIdent: types.NewPkgName(token.NoPos, nil, "def", types.NewPackage(defPkgPath, "def")),
			},
		},
	}
	method, err := parseQueryMethod(pkgForMethods, qFn, queryCall, map[string]*structInfo{})
	if err != nil {
		t.Fatalf("parseQueryMethod() error = %v", err)
	}
	if len(method.ParamTypes) != 1 {
		t.Fatalf("parseQueryMethod() should include unnamed param type entry")
	}

	mutationCall := makeDefCall(defIdent, "Create", ast.NewIdent("missing"))
	mFn := &ast.FuncDecl{
		Name: ast.NewIdent("CreateOne"),
		Recv: &ast.FieldList{List: []*ast.Field{{Type: &ast.StarExpr{X: ast.NewIdent("repo")}}}},
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					{Type: ast.NewIdent("int")},
				},
			},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{Type: ast.NewIdent("any")},
				},
			},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: mutationCall}}},
	}
	_, err = parseMutationMethod(pkgForMethods, mFn, MethodKindCreate, mutationCall, map[string]*structInfo{})
	if err == nil || !strings.Contains(err.Error(), "unknown identifier in Create") {
		t.Fatalf("parseMutationMethod() error = %v, want create identifier error", err)
	}
}

func TestParseExpressionErrorPropagationBranches(t *testing.T) {
	ctx := newParseTestContext()

	_, err := parseFilterOperand(ctx.pkg, makeDefGenericCall(ctx.defIdent, "Func", ast.NewIdent("string"),
		&ast.BasicLit{Kind: token.STRING, Value: `"LOWER"`},
		ast.NewIdent("missing"),
	), ctx.structs, nil, false)
	if err == nil || !strings.Contains(err.Error(), "unknown identifier") {
		t.Fatalf("parseFilterOperand(def.Func bad arg) error = %v", err)
	}

	_, err = buildSetValueExprSQL(ctx.pkg, &ast.ParenExpr{X: ast.NewIdent("missing")}, ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "unknown identifier") {
		t.Fatalf("buildSetValueExprSQL(paren) error = %v", err)
	}

	_, err = buildSetValueExprSQL(ctx.pkg, &ast.UnaryExpr{Op: token.ADD, X: ast.NewIdent("missing")}, ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "unknown identifier") {
		t.Fatalf("buildSetValueExprSQL(unary inner) error = %v", err)
	}

	_, err = buildSetValueExprSQL(ctx.pkg, &ast.CallExpr{Fun: ast.NewIdent("custom"), Args: []ast.Expr{ast.NewIdent("missing")}}, ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "unknown identifier in Set value expression") {
		t.Fatalf("buildSetValueExprSQL(call arg error) error = %v", err)
	}
}

func TestParseQueryArgsAndColumnExprErrorBranches(t *testing.T) {
	ctx := newParseTestContext()

	_, _, _, _, err := parseQueryArgs(ctx.pkg, &ast.CallExpr{
		Args: []ast.Expr{
			makeDefCall(ctx.defIdent, "Filter", ast.NewIdent("a"), ast.NewIdent("b")),
		},
	}, ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "def.Filter requires exactly 1 argument") {
		t.Fatalf("parseQueryArgs(filter argc) error = %v", err)
	}

	_, _, _, _, err = parseQueryArgs(ctx.pkg, &ast.CallExpr{
		Args: []ast.Expr{
			makeDefCall(ctx.defIdent, "Limit", ast.NewIdent("bad")),
		},
	}, ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to parse Limit") {
		t.Fatalf("parseQueryArgs(limit error) = %v", err)
	}

	_, _, _, _, err = parseQueryArgs(ctx.pkg, &ast.CallExpr{
		Args: []ast.Expr{
			makeDefCall(ctx.defIdent, "Offset", ast.NewIdent("bad")),
		},
	}, ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to parse Offset") {
		t.Fatalf("parseQueryArgs(offset error) = %v", err)
	}

	_, err = parseColumnExpr(ctx.pkg, makeDefGenericCall(ctx.defIdent, "Count", ast.NewIdent("int64")), ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "requires 1 argument") {
		t.Fatalf("parseColumnExpr(count argc) = %v", err)
	}

	_, err = parseColumnExpr(ctx.pkg, makeDefGenericCall(ctx.defIdent, "Count", ast.NewIdent("int64"), ast.NewIdent("x")), ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to parse Count argument") {
		t.Fatalf("parseColumnExpr(count arg type) = %v", err)
	}

	_, err = parseColumnExpr(ctx.pkg, &ast.BasicLit{Kind: token.INT, Value: "1"}, ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported column expression type") {
		t.Fatalf("parseColumnExpr(unsupported expr) = %v", err)
	}

	_, err = parseFuncExpr(ctx.pkg, makeDefGenericCall(ctx.defIdent, "Func", ast.NewIdent("string")), ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "requires at least 1 argument") {
		t.Fatalf("parseFuncExpr(no args) = %v", err)
	}

	_, err = parseFuncExpr(ctx.pkg, makeDefGenericCall(ctx.defIdent, "Func", ast.NewIdent("string"), ast.NewIdent("name")), ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "first argument must be a string literal") {
		t.Fatalf("parseFuncExpr(non string name) = %v", err)
	}
}

func TestParseFilterRecursiveAndLeafErrorBranches(t *testing.T) {
	ctx := newParseTestContext()

	_, err := parseFilterExprRecursive(ctx.pkg, &ast.CallExpr{Fun: ast.NewIdent("custom")}, ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported call expression in filter") {
		t.Fatalf("parseFilterExprRecursive(unsupported call) = %v", err)
	}

	_, err = parseFilterExprRecursive(ctx.pkg, ast.NewIdent("x"), ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported filter expression type") {
		t.Fatalf("parseFilterExprRecursive(unsupported type) = %v", err)
	}

	landErrExpr := &ast.BinaryExpr{
		X:  ast.NewIdent("x"),
		Op: token.LAND,
		Y:  &ast.BinaryExpr{X: &ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("ID")}, Op: token.EQL, Y: &ast.BasicLit{Kind: token.INT, Value: "1"}},
	}
	_, err = parseFilterExprRecursive(ctx.pkg, landErrExpr, ctx.structs, nil)
	if err == nil {
		t.Fatalf("parseFilterExprRecursive(land left error) expected error")
	}

	lorErrExpr := &ast.BinaryExpr{
		X:  &ast.BinaryExpr{X: &ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("ID")}, Op: token.EQL, Y: &ast.BasicLit{Kind: token.INT, Value: "1"}},
		Op: token.LOR,
		Y:  ast.NewIdent("x"),
	}
	_, err = parseFilterExprRecursive(ctx.pkg, lorErrExpr, ctx.structs, nil)
	if err == nil {
		t.Fatalf("parseFilterExprRecursive(lor right error) expected error")
	}

	_, err = parseInExpr(ctx.pkg, makeDefCall(ctx.defIdent, "In", &ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("ID")}), ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "requires 2 arguments") {
		t.Fatalf("parseInExpr(argc) = %v", err)
	}

	_, err = parseIsNullExpr(ctx.pkg, makeDefCall(ctx.defIdent, "IsNull", ast.NewIdent("a"), ast.NewIdent("b")), ctx.structs, nil, token.EQL)
	if err == nil || !strings.Contains(err.Error(), "requires exactly 1 argument") {
		t.Fatalf("parseIsNullExpr(argc) = %v", err)
	}

	_, err = parseIsNullExpr(ctx.pkg, makeDefCall(ctx.defIdent, "IsNull", ast.NewIdent("missing")), ctx.structs, nil, token.EQL)
	if err == nil || !strings.Contains(err.Error(), "failed to parse IsNull field") {
		t.Fatalf("parseIsNullExpr(bad operand) = %v", err)
	}

	_, err = parseComparisonExpr(ctx.pkg, &ast.BinaryExpr{X: ast.NewIdent("missing"), Op: token.EQL, Y: ast.NewIdent("x")}, ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to parse left operand") {
		t.Fatalf("parseComparisonExpr(left) = %v", err)
	}

	_, err = parseComparisonExpr(ctx.pkg, &ast.BinaryExpr{
		X:  &ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("ID")},
		Op: token.EQL,
		Y:  ast.NewIdent("missing"),
	}, ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to parse right operand") {
		t.Fatalf("parseComparisonExpr(right) = %v", err)
	}
}
