package defgen

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

func TestParseDetectorNegativeBranches(t *testing.T) {
	ctx := newParseTestContext()

	varIdent := ast.NewIdent("v")
	otherPkgIdent := ast.NewIdent("other")
	ctx.pkg.TypesInfo.Uses[varIdent] = types.NewVar(token.NoPos, nil, "v", types.Typ[types.Int])
	ctx.pkg.TypesInfo.Uses[otherPkgIdent] = types.NewPkgName(token.NoPos, nil, "other", types.NewPackage("example.com/other", "other"))

	if isDefCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   &ast.SelectorExpr{X: ast.NewIdent("x"), Sel: ast.NewIdent("y")},
			Sel: ast.NewIdent("Query"),
		},
	}, "Query") {
		t.Fatalf("isDefCall(non-ident selector root) expected false")
	}

	if isDefCall(ctx.pkg, makeDefCall(ast.NewIdent("missing"), "Query"), "Query") {
		t.Fatalf("isDefCall(missing pkg use) expected false")
	}

	if isDefCall(ctx.pkg, makeDefCall(varIdent, "Query"), "Query") {
		t.Fatalf("isDefCall(non-package object) expected false")
	}

	if _, ok := isGenericDefCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.IndexExpr{
			X: &ast.SelectorExpr{
				X:   &ast.SelectorExpr{X: ast.NewIdent("x"), Sel: ast.NewIdent("y")},
				Sel: ast.NewIdent("Count"),
			},
			Index: ast.NewIdent("int64"),
		},
	}); ok {
		t.Fatalf("isGenericDefCall(non-ident selector root) expected false")
	}

	if _, ok := isGenericDefCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.IndexExpr{
			X: &ast.SelectorExpr{
				X:   ast.NewIdent("missing"),
				Sel: ast.NewIdent("Count"),
			},
			Index: ast.NewIdent("int64"),
		},
	}); ok {
		t.Fatalf("isGenericDefCall(missing pkg use) expected false")
	}

	if _, ok := isGenericDefCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.IndexExpr{
			X: &ast.SelectorExpr{
				X:   varIdent,
				Sel: ast.NewIdent("Count"),
			},
			Index: ast.NewIdent("int64"),
		},
	}); ok {
		t.Fatalf("isGenericDefCall(non-package object) expected false")
	}

	if _, ok := isGenericDefCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.IndexExpr{
			X: &ast.SelectorExpr{
				X:   otherPkgIdent,
				Sel: ast.NewIdent("Count"),
			},
			Index: ast.NewIdent("int64"),
		},
	}); ok {
		t.Fatalf("isGenericDefCall(other package) expected false")
	}

	if isPostgresReturningCall(ctx.pkg, &ast.CallExpr{Fun: ast.NewIdent("x")}) {
		t.Fatalf("isPostgresReturningCall(non-selector) expected false")
	}
	if isPostgresReturningCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent("missing"),
			Sel: ast.NewIdent("Returning"),
		},
	}) {
		t.Fatalf("isPostgresReturningCall(missing pkg use) expected false")
	}
	if isPostgresReturningCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   varIdent,
			Sel: ast.NewIdent("Returning"),
		},
	}) {
		t.Fatalf("isPostgresReturningCall(non-package object) expected false")
	}

	if isPostgresExcludedCall(ctx.pkg, &ast.CallExpr{Fun: ast.NewIdent("x")}) {
		t.Fatalf("isPostgresExcludedCall(non-selector) expected false")
	}
	if isPostgresExcludedCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent("missing"),
			Sel: ast.NewIdent("Excluded"),
		},
	}) {
		t.Fatalf("isPostgresExcludedCall(missing pkg use) expected false")
	}
	if isPostgresExcludedCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   varIdent,
			Sel: ast.NewIdent("Excluded"),
		},
	}) {
		t.Fatalf("isPostgresExcludedCall(non-package object) expected false")
	}

	if isPostgresCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   &ast.SelectorExpr{X: ast.NewIdent("x"), Sel: ast.NewIdent("y")},
			Sel: ast.NewIdent("Now"),
		},
	}, "Now") {
		t.Fatalf("isPostgresCall(non-ident selector root) expected false")
	}
	if isPostgresCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent("missing"),
			Sel: ast.NewIdent("Now"),
		},
	}, "Now") {
		t.Fatalf("isPostgresCall(missing pkg use) expected false")
	}
	if isPostgresCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   varIdent,
			Sel: ast.NewIdent("Now"),
		},
	}, "Now") {
		t.Fatalf("isPostgresCall(non-package object) expected false")
	}
	if isPostgresCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   otherPkgIdent,
			Sel: ast.NewIdent("Now"),
		},
	}, "Now") {
		t.Fatalf("isPostgresCall(other package) expected false")
	}

	if _, _, ok := isPostgresOnConflictDoCall(ctx.pkg, &ast.CallExpr{Fun: ast.NewIdent("x")}); ok {
		t.Fatalf("isPostgresOnConflictDoCall(non-selector) expected false")
	}
	if _, _, ok := isPostgresOnConflictDoCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   makePostgresCall(ctx.pgIdent, "OnConflict"),
			Sel: ast.NewIdent("Unknown"),
		},
	}); ok {
		t.Fatalf("isPostgresOnConflictDoCall(unknown action) expected false")
	}
	if _, _, ok := isPostgresOnConflictDoCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent("x"),
			Sel: ast.NewIdent("DoNothing"),
		},
	}); ok {
		t.Fatalf("isPostgresOnConflictDoCall(non-call inner) expected false")
	}
	if _, _, ok := isPostgresOnConflictDoCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X: &ast.CallExpr{Fun: ast.NewIdent("x")},
			Sel: ast.NewIdent("DoNothing"),
		},
	}); ok {
		t.Fatalf("isPostgresOnConflictDoCall(non-selector inner fun) expected false")
	}
	if _, _, ok := isPostgresOnConflictDoCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ctx.pgIdent,
					Sel: ast.NewIdent("Returning"),
				},
			},
			Sel: ast.NewIdent("DoNothing"),
		},
	}); ok {
		t.Fatalf("isPostgresOnConflictDoCall(non-OnConflict inner selector) expected false")
	}
	if _, _, ok := isPostgresOnConflictDoCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   &ast.SelectorExpr{X: ast.NewIdent("x"), Sel: ast.NewIdent("y")},
					Sel: ast.NewIdent("OnConflict"),
				},
			},
			Sel: ast.NewIdent("DoNothing"),
		},
	}); ok {
		t.Fatalf("isPostgresOnConflictDoCall(non-ident package root) expected false")
	}
	if _, _, ok := isPostgresOnConflictDoCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("missing"),
					Sel: ast.NewIdent("OnConflict"),
				},
			},
			Sel: ast.NewIdent("DoNothing"),
		},
	}); ok {
		t.Fatalf("isPostgresOnConflictDoCall(missing package use) expected false")
	}
	if _, _, ok := isPostgresOnConflictDoCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   varIdent,
					Sel: ast.NewIdent("OnConflict"),
				},
			},
			Sel: ast.NewIdent("DoNothing"),
		},
	}); ok {
		t.Fatalf("isPostgresOnConflictDoCall(non-package object) expected false")
	}
	if _, _, ok := isPostgresOnConflictDoCall(ctx.pkg, &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   otherPkgIdent,
					Sel: ast.NewIdent("OnConflict"),
				},
			},
			Sel: ast.NewIdent("DoNothing"),
		},
	}); ok {
		t.Fatalf("isPostgresOnConflictDoCall(other package) expected false")
	}
}

func TestParseAdditionalBranches(t *testing.T) {
	ctx := newParseTestContext()

	if got := getReceiverTypeName(&ast.ArrayType{}); got != "" {
		t.Fatalf("getReceiverTypeName(array) = %q, want empty", got)
	}

	_, _, _, _, err := parseQueryArgs(ctx.pkg, &ast.CallExpr{
		Args: []ast.Expr{
			makeDefCall(ctx.defIdent, "Column", &ast.BasicLit{Kind: token.INT, Value: "1"}),
		},
	}, ctx.structs, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to parse column") {
		t.Fatalf("parseQueryArgs(column error) = %v", err)
	}

	if _, err := parseFuncExpr(ctx.pkg, makeDefGenericCall(
		ctx.defIdent,
		"Func",
		ast.NewIdent("string"),
		&ast.BasicLit{Kind: token.STRING, Value: `"LOWER"`},
		ast.NewIdent("missing"),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "failed to parse function argument") {
		t.Fatalf("parseFuncExpr(func arg error) = %v", err)
	}

	if _, err := parseFuncArg(ctx.pkg, &ast.SelectorExpr{
		X:   &ast.CallExpr{Fun: ast.NewIdent("x")},
		Sel: ast.NewIdent("ID"),
	}, ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "unexpected expression type in field path") {
		t.Fatalf("parseFuncArg(selector bad root) = %v", err)
	}

	filter, err := parseFilterExprRecursive(ctx.pkg, &ast.ParenExpr{
		X: &ast.BinaryExpr{
			X:  &ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("ID")},
			Op: token.EQL,
			Y:  &ast.BasicLit{Kind: token.INT, Value: "1"},
		},
	}, ctx.structs, nil)
	if err != nil || filter == nil || filter.Kind != FilterComparison {
		t.Fatalf("parseFilterExprRecursive(paren) = (%+v, %v)", filter, err)
	}

	if _, err := parseInExpr(ctx.pkg, makeDefCall(ctx.defIdent, "In",
		ast.NewIdent("missing"),
		ast.NewIdent("ids"),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "failed to parse In field") {
		t.Fatalf("parseInExpr(field error) = %v", err)
	}

	if _, err := parseInExpr(ctx.pkg, makeDefCall(ctx.defIdent, "In",
		&ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("ID")},
		&ast.CallExpr{Fun: ast.NewIdent("custom")},
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "failed to parse In values") {
		t.Fatalf("parseInExpr(values error) = %v", err)
	}

	got, err := parseFilterOperand(ctx.pkg, makeDefGenericCall(
		ctx.defIdent,
		"Count",
		ast.NewIdent("int64"),
		&ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("ID")},
	), ctx.structs, nil, false)
	if err != nil || !got.IsFunc || len(got.FuncArgs) != 1 {
		t.Fatalf("parseFilterOperand(valid count) = (%+v, %v)", got, err)
	}

	got, err = parseFilterOperand(ctx.pkg, makeDefGenericCall(
		ctx.defIdent,
		"Func",
		ast.NewIdent("string"),
		&ast.BasicLit{Kind: token.STRING, Value: `"COALESCE"`},
		&ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("Name")},
		&ast.BasicLit{Kind: token.STRING, Value: `"x"`},
	), ctx.structs, nil, false)
	if err != nil || !got.IsFunc || got.FuncName != "COALESCE" || len(got.FuncArgs) != 2 {
		t.Fatalf("parseFilterOperand(valid func) = (%+v, %v)", got, err)
	}

	if _, err := parseFieldPath(ctx.pkg, &ast.SelectorExpr{
		X:   &ast.CallExpr{Fun: ast.NewIdent("x")},
		Sel: ast.NewIdent("ID"),
	}, ctx.structs); err == nil || !strings.Contains(err.Error(), "unexpected expression type in field path") {
		t.Fatalf("parseFieldPath(unexpected root expr) = %v", err)
	}

	defIdent := ast.NewIdent("def")
	mutationCall := makeDefCall(defIdent, "Create", ast.NewIdent("missing"))
	pkgForMethods := &Package{
		Syntax: []*ast.File{
			{
				Decls: []ast.Decl{
					&ast.FuncDecl{
						Name: ast.NewIdent("CreateBad"),
						Recv: &ast.FieldList{List: []*ast.Field{{Type: &ast.StarExpr{X: ast.NewIdent("repo")}}}},
						Type: &ast.FuncType{},
						Body: &ast.BlockStmt{
							List: []ast.Stmt{
								&ast.ExprStmt{X: mutationCall},
							},
						},
					},
				},
			},
		},
		TypesInfo: &types.Info{
			Uses: map[*ast.Ident]types.Object{
				defIdent: types.NewPkgName(token.NoPos, nil, "def", types.NewPackage(defPkgPath, "def")),
			},
		},
	}
	if err := parseMutationMethods(pkgForMethods, map[string]*structInfo{}); err == nil || !strings.Contains(err.Error(), "failed to parse method CreateBad") {
		t.Fatalf("parseMutationMethods(wrapper error) = %v", err)
	}
}

func TestParseCreateUpdateDeleteAdditionalErrors(t *testing.T) {
	ctx := newParseTestContext()

	if _, err := parseCreateArgs(ctx.pkg, makeDefCall(ctx.defIdent, "Create", &ast.BasicLit{
		Kind:  token.INT,
		Value: "1",
	}), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "entity mode requires an identifier") {
		t.Fatalf("parseCreateArgs(non-ident entity mode) = %v", err)
	}

	validSet := makeDefCall(ctx.defIdent, "Set",
		&ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("Name")},
		&ast.BasicLit{Kind: token.STRING, Value: `"x"`},
	)

	if _, err := parseCreateArgs(ctx.pkg, makeDefCall(ctx.defIdent, "Create",
		validSet,
		makePostgresCall(ctx.pgIdent, "Returning", ast.NewIdent("bad")),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "failed to parse Returning column") {
		t.Fatalf("parseCreateArgs(field mode returning error) = %v", err)
	}

	badOnConflict := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   makePostgresCall(ctx.pgIdent, "OnConflict"),
			Sel: ast.NewIdent("DoNothing"),
		},
	}
	if _, err := parseCreateArgs(ctx.pkg, makeDefCall(ctx.defIdent, "Create",
		validSet,
		badOnConflict,
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "requires at least 1 column argument") {
		t.Fatalf("parseCreateArgs(field mode on conflict error) = %v", err)
	}

	if _, err := parseCreateArgs(ctx.pkg, makeDefCall(ctx.defIdent, "Create",
		makeDefCall(ctx.defIdent, "Set", ast.NewIdent("x")),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "requires exactly 2 arguments") {
		t.Fatalf("parseCreateArgs(field mode set parse error) = %v", err)
	}

	params := []ParamInfo{{Name: "u", Type: types.NewPointer(ctx.userType)}}
	if _, err := parseCreateArgs(ctx.pkg, makeDefCall(ctx.defIdent, "Create",
		ast.NewIdent("u"),
		makePostgresCall(ctx.pgIdent, "Returning", ast.NewIdent("bad")),
	), ctx.structs, params); err == nil || !strings.Contains(err.Error(), "failed to parse Returning column") {
		t.Fatalf("parseCreateArgs(entity mode returning error) = %v", err)
	}

	if _, err := parseCreateArgs(ctx.pkg, makeDefCall(ctx.defIdent, "Create",
		ast.NewIdent("u"),
		badOnConflict,
	), ctx.structs, params); err == nil || !strings.Contains(err.Error(), "requires at least 1 column argument") {
		t.Fatalf("parseCreateArgs(entity mode on conflict error) = %v", err)
	}

	if _, err := parseUpdateArgs(ctx.pkg, makeDefCall(ctx.defIdent, "Update"), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "requires at least 1 argument") {
		t.Fatalf("parseUpdateArgs(no args) = %v", err)
	}

	if _, err := parseUpdateArgs(ctx.pkg, makeDefCall(ctx.defIdent, "Update",
		ast.NewIdent("u"),
	), ctx.structs, params); err == nil || !strings.Contains(err.Error(), "requires at least one Filter expression") {
		t.Fatalf("parseUpdateArgs(entity mode no filter) = %v", err)
	}

	if _, err := parseUpdateArgs(ctx.pkg, makeDefCall(ctx.defIdent, "Update",
		ast.NewIdent("u"),
		makeDefCall(ctx.defIdent, "Filter", ast.NewIdent("a"), ast.NewIdent("b")),
	), ctx.structs, params); err == nil || !strings.Contains(err.Error(), "requires exactly 1 argument") {
		t.Fatalf("parseUpdateArgs(entity mode extra arg error) = %v", err)
	}

	if _, err := parseUpdateArgs(ctx.pkg, makeDefCall(ctx.defIdent, "Update",
		makeDefCall(ctx.defIdent, "Set", ast.NewIdent("x")),
		makeDefCall(ctx.defIdent, "Filter", &ast.BinaryExpr{
			X:  &ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("ID")},
			Op: token.EQL,
			Y:  &ast.BasicLit{Kind: token.INT, Value: "1"},
		}),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "requires exactly 2 arguments") {
		t.Fatalf("parseUpdateArgs(field mode set parse error) = %v", err)
	}

	if _, err := parseUpdateArgs(ctx.pkg, makeDefCall(ctx.defIdent, "Update",
		validSet,
		makeDefCall(ctx.defIdent, "Filter", ast.NewIdent("x"), ast.NewIdent("y")),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "requires exactly 1 argument") {
		t.Fatalf("parseUpdateArgs(field mode extra arg parse error) = %v", err)
	}

	if _, err := parseUpdateArgs(ctx.pkg, makeDefCall(ctx.defIdent, "Update",
		makeDefCall(ctx.defIdent, "Filter", &ast.BinaryExpr{
			X:  &ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("ID")},
			Op: token.EQL,
			Y:  &ast.BasicLit{Kind: token.INT, Value: "1"},
		}),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "requires at least one Set expression") {
		t.Fatalf("parseUpdateArgs(field mode no set) = %v", err)
	}

	if _, err := parseUpdateArgs(ctx.pkg, makeDefCall(ctx.defIdent, "Update",
		validSet,
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "requires at least one Filter expression") {
		t.Fatalf("parseUpdateArgs(field mode no filter) = %v", err)
	}

	updateResult := &updateArgsResult{}
	if err := parseUpdateExtraArg(ctx.pkg, makePostgresCall(ctx.pgIdent, "Returning", ast.NewIdent("bad")), ctx.structs, nil, updateResult); err == nil || !strings.Contains(err.Error(), "failed to parse Returning column") {
		t.Fatalf("parseUpdateExtraArg(returning parse error) = %v", err)
	}
	if err := parseUpdateExtraArg(ctx.pkg, makeDefCall(ctx.defIdent, "Filter",
		ast.NewIdent("x"),
		ast.NewIdent("y"),
	), ctx.structs, nil, updateResult); err == nil || !strings.Contains(err.Error(), "requires exactly 1 argument") {
		t.Fatalf("parseUpdateExtraArg(filter argc) = %v", err)
	}
	if err := parseUpdateExtraArg(ctx.pkg, makeDefCall(ctx.defIdent, "Filter",
		ast.NewIdent("x"),
	), ctx.structs, nil, updateResult); err == nil || !strings.Contains(err.Error(), "unsupported filter expression type") {
		t.Fatalf("parseUpdateExtraArg(filter parse error) = %v", err)
	}
	if err := parseUpdateExtraArg(ctx.pkg, makeDefCall(ctx.defIdent, "With"), ctx.structs, nil, updateResult); err == nil || !strings.Contains(err.Error(), "requires a name and at least one clause") {
		t.Fatalf("parseUpdateExtraArg(with parse error) = %v", err)
	}
	if err := parseUpdateExtraArg(ctx.pkg, makeDefCall(ctx.defIdent, "From", ast.NewIdent("x")), ctx.structs, nil, updateResult); err == nil || !strings.Contains(err.Error(), "requires a source name string literal") {
		t.Fatalf("parseUpdateExtraArg(from parse error) = %v", err)
	}
	if err := parseUpdateExtraArg(ctx.pkg, makeDefCall(ctx.defIdent, "Column", &ast.SelectorExpr{
		X:   ctx.userIdent,
		Sel: ast.NewIdent("ID"),
	}), ctx.structs, nil, updateResult); err != nil {
		t.Fatalf("parseUpdateExtraArg(unknown extra call) = %v", err)
	}

	if _, err := parseWithClause(ctx.pkg, makeDefCall(ctx.defIdent, "With",
		&ast.BasicLit{Kind: token.STRING, Value: "\"broken"},
		makeDefCall(ctx.defIdent, "From", ctx.userIdent),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "invalid def.With name") {
		t.Fatalf("parseWithClause(invalid name literal) = %v", err)
	}

	withName := &ast.BasicLit{Kind: token.STRING, Value: `"c"`}
	if _, err := parseWithClause(ctx.pkg, makeDefCall(ctx.defIdent, "With",
		withName,
		makeDefCall(ctx.defIdent, "Column", ast.NewIdent("x"), ast.NewIdent("y")),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "def.Column requires exactly 1 argument") {
		t.Fatalf("parseWithClause(column argc) = %v", err)
	}
	if _, err := parseWithClause(ctx.pkg, makeDefCall(ctx.defIdent, "With",
		withName,
		makeDefCall(ctx.defIdent, "Column", &ast.BasicLit{Kind: token.INT, Value: "1"}),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "failed to parse def.With column") {
		t.Fatalf("parseWithClause(column parse error) = %v", err)
	}
	if _, err := parseWithClause(ctx.pkg, makeDefCall(ctx.defIdent, "With",
		withName,
		makeDefCall(ctx.defIdent, "Filter", ast.NewIdent("x"), ast.NewIdent("y")),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "def.Filter requires exactly 1 argument") {
		t.Fatalf("parseWithClause(filter argc) = %v", err)
	}
	if _, err := parseWithClause(ctx.pkg, makeDefCall(ctx.defIdent, "With",
		withName,
		makeDefCall(ctx.defIdent, "Filter", ast.NewIdent("x")),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "failed to parse def.With filter") {
		t.Fatalf("parseWithClause(filter parse error) = %v", err)
	}
	if _, err := parseWithClause(ctx.pkg, makeDefCall(ctx.defIdent, "With",
		withName,
		makeDefCall(ctx.defIdent, "From", ast.NewIdent("missing")),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "unable to infer source type") {
		t.Fatalf("parseWithClause(from parse error) = %v", err)
	}
	if _, err := parseWithClause(ctx.pkg, makeDefCall(ctx.defIdent, "With",
		withName,
		makeDefCall(ctx.defIdent, "From", ctx.userIdent),
		makeDefCall(ctx.defIdent, "OrderBy", makeDefCall(ctx.defIdent, "Asc")),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "requires exactly 1 argument") {
		t.Fatalf("parseWithClause(order by parse error) = %v", err)
	}
	if _, err := parseWithClause(ctx.pkg, makeDefCall(ctx.defIdent, "With",
		withName,
		makeDefCall(ctx.defIdent, "From", ctx.userIdent),
		makeDefCall(ctx.defIdent, "Limit", ast.NewIdent("bad")),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "failed to parse def.With limit") {
		t.Fatalf("parseWithClause(limit parse error) = %v", err)
	}
	if _, err := parseWithClause(ctx.pkg, makeDefCall(ctx.defIdent, "With",
		withName,
		makeDefCall(ctx.defIdent, "From", ctx.userIdent),
		makeDefCall(ctx.defIdent, "Offset", ast.NewIdent("bad")),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "failed to parse def.With offset") {
		t.Fatalf("parseWithClause(offset parse error) = %v", err)
	}
	if _, err := parseWithClause(ctx.pkg, makeDefCall(ctx.defIdent, "With",
		withName,
		makeDefCall(ctx.defIdent, "From", ctx.userIdent),
		makeDefCall(ctx.defIdent, "Query"),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "unsupported def.With argument") {
		t.Fatalf("parseWithClause(unsupported argument) = %v", err)
	}

	if _, err := parseOrderByExprs(ctx.pkg, makeDefCall(ctx.defIdent, "OrderBy", ast.NewIdent("bad")), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "failed to parse ORDER BY expression") {
		t.Fatalf("parseOrderByExprs(item parse error) = %v", err)
	}

	if _, _, _, err := parseDeleteArgs(ctx.pkg, makeDefCall(ctx.defIdent, "Delete",
		ast.NewIdent("skip"),
		makePostgresCall(ctx.pgIdent, "Returning", ast.NewIdent("bad")),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "failed to parse Returning column") {
		t.Fatalf("parseDeleteArgs(returning parse error) = %v", err)
	}
	if _, _, _, err := parseDeleteArgs(ctx.pkg, makeDefCall(ctx.defIdent, "Delete",
		makeDefCall(ctx.defIdent, "Filter", ast.NewIdent("a"), ast.NewIdent("b")),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "def.Filter requires exactly 1 argument") {
		t.Fatalf("parseDeleteArgs(filter argc) = %v", err)
	}
	if _, _, _, err := parseDeleteArgs(ctx.pkg, makeDefCall(ctx.defIdent, "Delete",
		makeDefCall(ctx.defIdent, "Filter", ast.NewIdent("x")),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "unsupported filter expression type") {
		t.Fatalf("parseDeleteArgs(filter parse error) = %v", err)
	}
}

func TestSetValueAndPostgresBranchExtensions(t *testing.T) {
	ctx := newParseTestContext()

	if _, err := parseSetExpr(ctx.pkg, makeDefCall(ctx.defIdent, "Set",
		&ast.SelectorExpr{
			X:   &ast.CallExpr{Fun: ast.NewIdent("x")},
			Sel: ast.NewIdent("Name"),
		},
		&ast.BasicLit{Kind: token.STRING, Value: `"x"`},
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "failed to parse Set field") {
		t.Fatalf("parseSetExpr(field parse error) = %v", err)
	}
	if _, err := parseSetExpr(ctx.pkg, makeDefCall(ctx.defIdent, "Set",
		&ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("Name")},
		ast.NewIdent("missing"),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "failed to parse Set value") {
		t.Fatalf("parseSetExpr(value parse error) = %v", err)
	}

	sql, err := buildSetValueExprSQL(ctx.pkg, &ast.UnaryExpr{
		Op: token.SUB,
		X: &ast.BinaryExpr{
			X:  &ast.BasicLit{Kind: token.INT, Value: "1"},
			Op: token.ADD,
			Y:  &ast.BasicLit{Kind: token.INT, Value: "2"},
		},
	}, ctx.structs, nil)
	if err != nil || sql != "-(1 + 2)" {
		t.Fatalf("buildSetValueExprSQL(unary over binary) = (%q, %v)", sql, err)
	}

	sql, err = buildSetValueExprSQL(ctx.pkg, &ast.BinaryExpr{
		X: &ast.BinaryExpr{
			X:  &ast.BasicLit{Kind: token.INT, Value: "1"},
			Op: token.ADD,
			Y:  &ast.BasicLit{Kind: token.INT, Value: "2"},
		},
		Op: token.MUL,
		Y: &ast.BinaryExpr{
			X:  &ast.BasicLit{Kind: token.INT, Value: "3"},
			Op: token.ADD,
			Y:  &ast.BasicLit{Kind: token.INT, Value: "4"},
		},
	}, ctx.structs, nil)
	if err != nil || sql != "(1 + 2) * (3 + 4)" {
		t.Fatalf("buildSetValueExprSQL(nested binary) = (%q, %v)", sql, err)
	}

	if _, err := buildSetValueExprSQL(ctx.pkg, makePostgresCall(ctx.pgIdent, "Interval", &ast.BasicLit{
		Kind:  token.STRING,
		Value: "\"bad",
	}), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "invalid postgres.Interval value") {
		t.Fatalf("buildSetValueExprSQL(interval invalid string) = %v", err)
	}

	if _, err := buildSetValueExprSQL(ctx.pkg, makePostgresCall(ctx.pgIdent, "Excluded",
		&ast.SelectorExpr{
			X:   &ast.CallExpr{Fun: ast.NewIdent("x")},
			Sel: ast.NewIdent("ID"),
		},
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "failed to parse postgres.Excluded field") {
		t.Fatalf("buildSetValueExprSQL(excluded field parse error) = %v", err)
	}

	sql, err = buildSetValueExprSQL(ctx.pkg, &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent("math"),
			Sel: ast.NewIdent("abs"),
		},
		Args: []ast.Expr{
			&ast.BasicLit{Kind: token.INT, Value: "1"},
		},
	}, ctx.structs, nil)
	if err != nil || sql != "math.abs(1)" {
		t.Fatalf("buildSetValueExprSQL(custom selector call) = (%q, %v)", sql, err)
	}

	if _, err := buildSetValueExprSQL(ctx.pkg, &ast.ArrayType{}, ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "unsupported Set value type") {
		t.Fatalf("buildSetValueExprSQL(unsupported expr type) = %v", err)
	}

	if name, err := setCallName(&ast.SelectorExpr{
		X:   &ast.CallExpr{Fun: ast.NewIdent("x")},
		Sel: ast.NewIdent("Now"),
	}); err != nil || name != "Now" {
		t.Fatalf("setCallName(selector fallback) = (%q, %v), want (Now,nil)", name, err)
	}

	if name, err := setCallName(&ast.IndexListExpr{X: ast.NewIdent("fn")}); err != nil || name != "fn" {
		t.Fatalf("setCallName(index list) = (%q, %v), want (fn,nil)", name, err)
	}

	if _, err := buildSetDefFuncSQL(ctx.pkg, makeDefGenericCall(
		ctx.defIdent,
		"Func",
		ast.NewIdent("any"),
		ast.NewIdent("name"),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "first argument must be a string literal") {
		t.Fatalf("buildSetDefFuncSQL(non-string name) = %v", err)
	}

	if _, err := buildSetDefFuncSQL(ctx.pkg, makeDefGenericCall(
		ctx.defIdent,
		"Func",
		ast.NewIdent("any"),
		&ast.BasicLit{Kind: token.STRING, Value: "\"bad"},
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "invalid def.Func function name") {
		t.Fatalf("buildSetDefFuncSQL(invalid name literal) = %v", err)
	}

	if _, err := buildSetDefFuncSQL(ctx.pkg, makeDefGenericCall(
		ctx.defIdent,
		"Func",
		ast.NewIdent("any"),
		&ast.BasicLit{Kind: token.STRING, Value: `"LOWER"`},
		ast.NewIdent("missing"),
	), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "unknown identifier in Set value expression") {
		t.Fatalf("buildSetDefFuncSQL(arg parse error) = %v", err)
	}

	inner := makePostgresCall(ctx.pgIdent, "OnConflict", &ast.SelectorExpr{X: ctx.userIdent, Sel: ast.NewIdent("ID")})
	outer := &ast.CallExpr{
		Args: []ast.Expr{
			ast.NewIdent("skip"),
			makeDefCall(ctx.defIdent, "Set", ast.NewIdent("x")),
		},
	}
	if _, _, err := parseOnConflictExpr(ctx.pkg, outer, inner, "update", ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "failed to parse DoUpdate Set") {
		t.Fatalf("parseOnConflictExpr(update set parse error) = %v", err)
	}

	if got := analyzeMutationReturnType(types.NewMap(types.Typ[types.String], types.Typ[types.Int])); got == nil || got.IsScalar || got.StructName != "" {
		t.Fatalf("analyzeMutationReturnType(map) = %+v, want non-nil non-scalar unnamed", got)
	}

	if _, err := parseReturningColumns(ctx.pkg, makePostgresCall(ctx.pgIdent, "Returning", ast.NewIdent("bad")), ctx.structs, nil); err == nil || !strings.Contains(err.Error(), "failed to parse Returning column") {
		t.Fatalf("parseReturningColumns(column parse error) = %v", err)
	}
}

func TestAnalyzeFilterAdditionalBranches(t *testing.T) {
	ctx := newParseTestContext()
	userBinding := ctx.pkg.Tables[getTypeKey(ctx.userType)]

	if _, err := analyzeFilterWithQualifier(ctx.pkg, &FilterExpr{
		Kind:     FilterAnd,
		Children: []*FilterExpr{{Kind: FilterComparison, Left: FilterOperand{IsField: true, FieldPath: []FieldPathElement{{VarName: "x"}}}}},
	}, nil); err == nil {
		t.Fatalf("analyzeFilterWithQualifier(and child error) expected error")
	}

	if _, err := analyzeFilterWithQualifier(ctx.pkg, &FilterExpr{
		Kind:     FilterOr,
		Children: []*FilterExpr{{Kind: FilterComparison, Left: FilterOperand{IsField: true, FieldPath: []FieldPathElement{{VarName: "x"}}}}},
	}, nil); err == nil {
		t.Fatalf("analyzeFilterWithQualifier(or child error) expected error")
	}

	if _, err := analyzeFilterWithQualifier(ctx.pkg, &FilterExpr{Kind: FilterKind(99)}, nil); err == nil || !strings.Contains(err.Error(), "unsupported filter kind") {
		t.Fatalf("analyzeFilterWithQualifier(unsupported kind) = %v", err)
	}

	if _, err := analyzeLeafFilter(ctx.pkg, &FilterExpr{
		Kind: FilterComparison,
		Left: FilterOperand{
			IsField: true,
			FieldPath: []FieldPathElement{
				{VarName: "ghost", Type: types.NewNamed(types.NewTypeName(token.NoPos, types.NewPackage("example.com/ghost", "ghost"), "Ghost", nil), types.NewStruct(nil, nil), nil)},
				{FieldName: "ID"},
			},
		},
		Right: FilterOperand{IsLiteral: true, LiteralKind: token.INT, LiteralValue: "1"},
	}, false, nil); err == nil || !strings.Contains(err.Error(), "failed to resolve field path in filter") {
		t.Fatalf("analyzeLeafFilter(unresolved simple column) = %v", err)
	}

	taskBinding := ctx.pkg.Tables[getTypeKey(ctx.taskType)]
	taskBinding.ForeignKeys = append(taskBinding.ForeignKeys, ForeignKeyInfo{
		FieldName: "Ghost",
		KeyColumn: "ghost_id",
		RefType:   types.NewNamed(types.NewTypeName(token.NoPos, types.NewPackage("example.com/ghost", "ghost"), "Ghost", nil), types.NewStruct(nil, nil), nil),
	})
	if _, err := analyzeLeafFilter(ctx.pkg, &FilterExpr{
		Kind: FilterComparison,
		Left: FilterOperand{
			IsField: true,
			FieldPath: []FieldPathElement{
				{VarName: "task", Type: ctx.taskType},
				{FieldName: "Ghost", IsForeignKey: true},
				{FieldName: "Name"},
			},
		},
		Right: FilterOperand{IsParam: true, ParamName: "name"},
	}, false, nil); err == nil || !strings.Contains(err.Error(), "table binding not found for type") {
		t.Fatalf("analyzeLeafFilter(ref table lookup error) = %v", err)
	}

	if _, err := analyzeLeafFilter(ctx.pkg, &FilterExpr{
		Kind: FilterComparison,
		Left: FilterOperand{
			IsField: true,
			FieldPath: []FieldPathElement{
				{VarName: "task", Type: ctx.taskType},
				{FieldName: "NoSuchFK", IsForeignKey: true},
				{FieldName: "Name"},
			},
		},
		Right: FilterOperand{IsParam: true, ParamName: "name"},
	}, false, nil); err == nil || !strings.Contains(err.Error(), "foreign key field") {
		t.Fatalf("analyzeLeafFilter(fk not matched) = %v", err)
	}

	originalPK := userBinding.PrimaryKey
	userBinding.PrimaryKey = nil
	if _, err := analyzeLeafFilter(ctx.pkg, &FilterExpr{
		Kind: FilterComparison,
		Left: FilterOperand{
			IsField: true,
			FieldPath: []FieldPathElement{
				{VarName: "task", Type: ctx.taskType},
				{FieldName: "User", IsForeignKey: true},
				{FieldName: "Name"},
			},
		},
		Right: FilterOperand{IsParam: true, ParamName: "name"},
	}, false, nil); err == nil || !strings.Contains(err.Error(), "has no primary key") {
		t.Fatalf("analyzeLeafFilter(ref pk missing) = %v", err)
	}
	userBinding.PrimaryKey = originalPK

	if _, err := analyzeLeafFilter(ctx.pkg, &FilterExpr{
		Kind: FilterComparison,
		Left: FilterOperand{
			IsField: true,
			FieldPath: []FieldPathElement{
				{VarName: "task", Type: ctx.taskType},
				{FieldName: "User", IsForeignKey: true},
				{FieldName: "Missing"},
			},
		},
		Right: FilterOperand{IsParam: true, ParamName: "name"},
	}, false, nil); err == nil || !strings.Contains(err.Error(), "field Missing not found") {
		t.Fatalf("analyzeLeafFilter(ref field missing) = %v", err)
	}

	analyzed, err := analyzeLeafFilter(ctx.pkg, &FilterExpr{
		Kind: FilterComparison,
		Left: FilterOperand{
			IsField: true,
			FieldPath: []FieldPathElement{
				{VarName: "task", Type: ctx.taskType},
				{FieldName: "User", IsForeignKey: true},
				{FieldName: "Name"},
			},
		},
		Right: FilterOperand{
			IsField: true,
			FieldPath: []FieldPathElement{
				{VarName: "user", Type: ctx.userType},
				{FieldName: "Name"},
			},
		},
	}, false, &filterQualifier{
		Default: "tasks",
		Vars: map[string]string{
			"task": "t",
			"user": "u",
		},
	})
	if err != nil || !analyzed.IsSubquery || analyzed.ForeignKeyCol != "t.user_id" || analyzed.SubqueryValue != "u.name" {
		t.Fatalf("analyzeLeafFilter(subquery with qualifier right field) = (%+v, %v)", analyzed, err)
	}

	if _, err := analyzeLeafFilter(ctx.pkg, &FilterExpr{
		Kind: FilterComparison,
		Left: FilterOperand{
			IsField: true,
			FieldPath: []FieldPathElement{
				{VarName: "task", Type: ctx.taskType},
				{FieldName: "User", IsForeignKey: true},
				{FieldName: "Name"},
			},
		},
		Op:    token.LSS,
		Right: FilterOperand{IsNil: true},
	}, false, nil); err == nil || !strings.Contains(err.Error(), "nil comparison only supports") {
		t.Fatalf("analyzeLeafFilter(nil op unsupported) = %v", err)
	}

	if got := formatFilterFuncArgWithQualifier(ctx.pkg, FuncArg{
		IsField: true,
		FieldPath: []FieldPathElement{
			{VarName: "user", Type: ctx.userType},
			{FieldName: "ID"},
		},
	}, &filterQualifier{
		Default: "users",
		Vars: map[string]string{
			"user": "u",
		},
	}); got != "u.id" {
		t.Fatalf("formatFilterFuncArgWithQualifier(field+qualifier) = %q, want u.id", got)
	}

	if got := resolveColumnNameFromPathWithQualifier(ctx.pkg, []FieldPathElement{{VarName: "x"}}, nil); got != "" {
		t.Fatalf("resolveColumnNameFromPathWithQualifier(short path) = %q, want empty", got)
	}
	if got := resolveColumnNameFromPathWithQualifier(ctx.pkg, []FieldPathElement{
		{VarName: "x", Type: types.Typ[types.Int]},
		{FieldName: "ID"},
	}, nil); got != "" {
		t.Fatalf("resolveColumnNameFromPathWithQualifier(unbound type) = %q, want empty", got)
	}

	if got := formatLiteral("1i", token.IMAG); got != "1i" {
		t.Fatalf("formatLiteral(default token) = %q, want 1i", got)
	}
}

func TestRelationAndSchemaAdditionalBranches(t *testing.T) {
	refType := namedStructType("example.com/demo", "demo", "Ref", []namedField{
		{name: "ID", typ: types.Typ[types.Int64], tag: `db:"id" primary_key:"true"`},
		{name: "Name", typ: types.Typ[types.String], tag: `db:"name"`},
	})
	sourceType := namedStructType("example.com/demo", "demo", "Source", []namedField{
		{name: "ID", typ: types.Typ[types.Int64], tag: `db:"id" primary_key:"true"`},
		{name: "RefID", typ: types.Typ[types.Int64], tag: `db:"ref_id"`},
		{name: "Ref", typ: types.NewPointer(refType), tag: `db:"-" foreign_key:"ref_id"`},
	})
	pkg := &Package{Tables: map[string]*TableBinding{}}
	addTableBindingFromNamed(pkg, sourceType, "sources")
	addTableBindingFromNamed(pkg, refType, "refs")
	sourceTable := pkg.Tables[getTypeKey(sourceType)]

	if got := analyzeBelongsTo(pkg, sourceTable, ForeignKeyInfo{RefType: types.Typ[types.Int]}); got != nil {
		t.Fatalf("analyzeBelongsTo(non-named ref type) = %+v, want nil", got)
	}
	if got := analyzeBelongsTo(pkg, sourceTable, ForeignKeyInfo{
		RefType: types.NewNamed(types.NewTypeName(token.NoPos, types.NewPackage("example.com/missing", "missing"), "Ghost", nil), types.NewStruct(nil, nil), nil),
	}); got != nil {
		t.Fatalf("analyzeBelongsTo(missing ref binding) = %+v, want nil", got)
	}

	if got := analyzeHasMany(pkg, sourceTable, ForeignKeyInfo{RefType: types.Typ[types.Int]}); got != nil {
		t.Fatalf("analyzeHasMany(non-named ref type) = %+v, want nil", got)
	}
	if got := analyzeHasMany(pkg, sourceTable, ForeignKeyInfo{
		RefType: types.NewNamed(types.NewTypeName(token.NoPos, types.NewPackage("example.com/missing", "missing"), "Ghost", nil), types.NewStruct(nil, nil), nil),
	}); got != nil {
		t.Fatalf("analyzeHasMany(missing ref binding) = %+v, want nil", got)
	}
	if got := analyzeHasMany(pkg, sourceTable, ForeignKeyInfo{
		RefType:   refType,
		KeyColumn: "not_found",
	}); got != nil {
		t.Fatalf("analyzeHasMany(missing fk field type) = %+v, want nil", got)
	}

	callbackMap := map[string]*CallbackMethod{}
	addCallbackField(callbackMap, sourceTable, ForeignKeyInfo{KeyColumn: "ref_id"}, nil)
	if len(callbackMap) != 0 {
		t.Fatalf("addCallbackField(nil belongsTo) should not modify callbackMap")
	}
	addCallbackField(callbackMap, sourceTable, ForeignKeyInfo{
		FieldName: "Ref",
		KeyColumn: "missing_key",
	}, &RelationMethod{
		MethodName:  "getRefByID",
		RefTypeName: "Ref",
	})
	if len(callbackMap) != 1 || len(callbackMap[sourceTable.TypeName].Fields) != 0 {
		t.Fatalf("addCallbackField(missing key field) should create callback but no fields: %+v", callbackMap[sourceTable.TypeName])
	}

	addHasManyCallbackField(pkg, callbackMap, sourceTable, ForeignKeyInfo{RefType: types.Typ[types.Int]})
	addHasManyCallbackField(pkg, callbackMap, sourceTable, ForeignKeyInfo{
		RefType: types.NewNamed(types.NewTypeName(token.NoPos, types.NewPackage("example.com/missing", "missing"), "Ghost", nil), types.NewStruct(nil, nil), nil),
	})

	nonStructRef := types.NewNamed(
		types.NewTypeName(token.NoPos, types.NewPackage("example.com/demo", "demo"), "Number", nil),
		types.Typ[types.Int],
		nil,
	)
	pkgNonStruct := &Package{Tables: map[string]*TableBinding{
		getTypeKey(nonStructRef): {
			Type:      nonStructRef,
			TypeName:  "Number",
			TableName: "numbers",
		},
	}}
	addHasManyCallbackField(pkgNonStruct, callbackMap, sourceTable, ForeignKeyInfo{
		RefType:   nonStructRef,
		KeyColumn: "ref_id",
	})

	refWithoutSlice := namedStructType("example.com/demo", "demo", "RefNoSlice", []namedField{
		{name: "ID", typ: types.Typ[types.Int64], tag: `db:"id" primary_key:"true"`},
	})
	pkgNoField := &Package{Tables: map[string]*TableBinding{}}
	addTableBindingFromNamed(pkgNoField, sourceType, "sources")
	addTableBindingFromNamed(pkgNoField, refWithoutSlice, "refs_no_slice")
	addHasManyCallbackField(pkgNoField, callbackMap, pkgNoField.Tables[getTypeKey(sourceType)], ForeignKeyInfo{
		RefType:   refWithoutSlice,
		KeyColumn: "ref_id",
	})

	refWithPtrField := namedStructType("example.com/demo", "demo", "RefWithPtr", []namedField{
		{name: "ID", typ: types.Typ[types.Int64], tag: `db:"id" primary_key:"true"`},
		{name: "Sources", typ: types.NewPointer(types.Typ[types.Int]), tag: `db:"-"`},
	})
	pkgPtr := &Package{Tables: map[string]*TableBinding{}}
	addTableBindingFromNamed(pkgPtr, sourceType, "sources")
	addTableBindingFromNamed(pkgPtr, refWithPtrField, "refs_ptr")
	addHasManyCallbackField(pkgPtr, callbackMap, pkgPtr.Tables[getTypeKey(sourceType)], ForeignKeyInfo{
		RefType:   refWithPtrField,
		KeyColumn: "ref_id",
	})

	refWithSlice := namedStructType("example.com/demo", "demo", "RefWithSlice", []namedField{
		{name: "ID", typ: types.Typ[types.Int64], tag: `db:"id" primary_key:"true"`},
		{name: "Sources", typ: types.NewSlice(types.NewPointer(sourceType)), tag: `db:"-"`},
	})
	pkgNoPK := &Package{Tables: map[string]*TableBinding{}}
	addTableBindingFromNamed(pkgNoPK, sourceType, "sources")
	addTableBindingFromNamed(pkgNoPK, refWithSlice, "refs_slice")
	pkgNoPK.Tables[getTypeKey(refWithSlice)].PrimaryKey = nil
	addHasManyCallbackField(pkgNoPK, callbackMap, pkgNoPK.Tables[getTypeKey(sourceType)], ForeignKeyInfo{
		RefType:   refWithSlice,
		KeyColumn: "ref_id",
	})

	if got := lcFirst(""); got != "" {
		t.Fatalf("lcFirst(empty) = %q, want empty", got)
	}
	if got := ucFirst(""); got != "" {
		t.Fatalf("ucFirst(empty) = %q, want empty", got)
	}
	if got := snakeToCamel("user_id"); got != "userID" {
		t.Fatalf("snakeToCamel(user_id) = %q, want userID", got)
	}

	st := types.NewStruct([]*types.Var{
		types.NewVar(token.NoPos, nil, "Ignored", types.Typ[types.String]),
	}, []string{`json:"ignored"`})
	if fields, fks := parseStructTags(st); len(fields) != 0 || len(fks) != 0 {
		t.Fatalf("parseStructTags(no db tag) = (%+v, %+v), want empty", fields, fks)
	}

	if got := typeStringForLookup(types.NewPointer(types.Typ[types.Int])); got == "" {
		t.Fatalf("typeStringForLookup(pointer basic) should not be empty")
	}

	fset := token.NewFileSet()
	pkgWithMissingObj := &Package{
		Fset: fset,
		Syntax: []*ast.File{
			{
				Name: ast.NewIdent("demo"),
				Decls: []ast.Decl{
					&ast.GenDecl{
						Tok: token.TYPE,
						Specs: []ast.Spec{
							&ast.TypeSpec{
								Name: ast.NewIdent("Ghost"),
								Type: &ast.StructType{Fields: &ast.FieldList{}},
							},
						},
					},
				},
			},
		},
		TypesPkg: types.NewPackage("example.com/demo", "demo"),
	}
	if got := parseAllStructs(pkgWithMissingObj); len(got) != 0 {
		t.Fatalf("parseAllStructs(missing scope obj) = %+v, want empty", got)
	}

	pkgWithNonNamedObj := &Package{
		Fset: fset,
		Syntax: []*ast.File{
			{
				Name: ast.NewIdent("demo"),
				Decls: []ast.Decl{
					&ast.GenDecl{
						Tok: token.TYPE,
						Specs: []ast.Spec{
							&ast.TypeSpec{
								Name: ast.NewIdent("NotNamed"),
								Type: &ast.StructType{Fields: &ast.FieldList{}},
							},
						},
					},
				},
			},
		},
		TypesPkg: types.NewPackage("example.com/demo", "demo"),
	}
	pkgWithNonNamedObj.TypesPkg.Scope().Insert(types.NewVar(token.NoPos, nil, "NotNamed", types.Typ[types.Int]))
	if got := parseAllStructs(pkgWithNonNamedObj); len(got) != 0 {
		t.Fatalf("parseAllStructs(non-named object) = %+v, want empty", got)
	}

	namedInt := types.NewNamed(types.NewTypeName(token.NoPos, types.NewPackage("example.com/demo", "demo"), "Value", nil), types.Typ[types.Int], nil)
	pkgWithNamedNonStruct := &Package{
		Fset: fset,
		Syntax: []*ast.File{
			{
				Name: ast.NewIdent("demo"),
				Decls: []ast.Decl{
					&ast.GenDecl{
						Tok: token.TYPE,
						Specs: []ast.Spec{
							&ast.TypeSpec{
								Name: ast.NewIdent("Value"),
								Type: &ast.StructType{Fields: &ast.FieldList{}},
							},
						},
					},
				},
			},
		},
		TypesPkg: types.NewPackage("example.com/demo", "demo"),
	}
	pkgWithNamedNonStruct.TypesPkg.Scope().Insert(namedInt.Obj())
	if got := parseAllStructs(pkgWithNamedNonStruct); len(got) != 0 {
		t.Fatalf("parseAllStructs(non-struct named type) = %+v, want empty", got)
	}

	if got := getStructInfoFromType(types.Typ[types.Int]); got != nil {
		t.Fatalf("getStructInfoFromType(non-named) = %+v, want nil", got)
	}
	if got := getStructInfoFromType(namedInt); got != nil {
		t.Fatalf("getStructInfoFromType(named non-struct) = %+v, want nil", got)
	}

	pkgInterfaces := &Package{
		Syntax: []*ast.File{
			{
				Name: ast.NewIdent("demo"),
				Decls: []ast.Decl{
					&ast.GenDecl{
						Tok: token.TYPE,
						Specs: []ast.Spec{
							&ast.TypeSpec{
								Name: ast.NewIdent("Store"),
								Type: &ast.InterfaceType{
									Methods: &ast.FieldList{
										List: []*ast.Field{
											{Type: ast.NewIdent("Embedded")},
											{
												Names: []*ast.Ident{ast.NewIdent("Broken")},
												Type:  ast.NewIdent("int"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		TypesInfo:  &types.Info{Types: map[ast.Expr]types.TypeAndValue{}},
		Interfaces: map[string]*InterfaceInfo{},
	}
	parseInterfaceDefs(pkgInterfaces)
	if info := pkgInterfaces.Interfaces["Store"]; info == nil || len(info.Methods) != 0 {
		t.Fatalf("parseInterfaceDefs(embedded+nonfunc fields) = %+v, want no methods", info)
	}
}

func TestSQLAndCodegenAdditionalBranches(t *testing.T) {
	pkg, userType := newSQLTestPackage()

	if got := resolveColumnName(pkg, []FieldPathElement{
		{VarName: "other", Type: types.Typ[types.Int]},
		{FieldName: "ID"},
	}); got != "" {
		t.Fatalf("resolveColumnName(unbound type) = %q, want empty", got)
	}
	if got := resolveColumnName(pkg, []FieldPathElement{
		{VarName: "user", Type: userType},
		{FieldName: "Missing"},
	}); got != "" {
		t.Fatalf("resolveColumnName(missing field) = %q, want empty", got)
	}

	if got := formatFilterTree(&AnalyzedFilter{
		Kind: AnalyzedFilterAnd,
		Children: []*AnalyzedFilter{
			nil,
		},
	}); got != "" {
		t.Fatalf("formatFilterTree(and all empty children) = %q, want empty", got)
	}
	if got := formatFilterTree(&AnalyzedFilter{
		Kind: AnalyzedFilterOr,
		Children: []*AnalyzedFilter{
			nil,
		},
	}); got != "" {
		t.Fatalf("formatFilterTree(or all empty children) = %q, want empty", got)
	}
	if got := formatFilterTree(&AnalyzedFilter{
		Kind:       AnalyzedFilterComparison,
		IsNull:     true,
		Operator:   "IS NULL",
		LeftIsFunc: true,
		LeftFuncExpr: "LOWER(name)",
	}); got != "LOWER(name) IS NULL" {
		t.Fatalf("formatFilterTree(is-null with left func) = %q", got)
	}
	if got := formatFilterTree(&AnalyzedFilter{Kind: AnalyzedFilterKind(99)}); got != "" {
		t.Fatalf("formatFilterTree(unknown kind) = %q, want empty", got)
	}

	if _, err := generateInsertSQL(pkg, &MutationMethod{
		Name:       "BadInsert",
		Kind:       MethodKindCreate,
		TargetType: getTypeKey(userType),
	}); err == nil || !strings.Contains(err.Error(), "no columns specified for INSERT") {
		t.Fatalf("generateInsertSQL(no sets) = %v", err)
	}

	if got := generateOnConflictClause(pkg, &MutationMethod{
		ConflictColumns: []ColumnExpr{
			{
				FieldPath: []FieldPathElement{
					{VarName: "user", Type: userType},
					{FieldName: "ID"},
				},
			},
		},
		ConflictAction: "update",
		ConflictSets: []SetExpr{
			{
				FieldPath: []FieldPathElement{
					{VarName: "user", Type: userType},
					{FieldName: "Missing"},
				},
				Value: SetValue{IsParam: true, ParamName: "v"},
			},
		},
	}); !strings.Contains(got, "DO UPDATE SET ") {
		t.Fatalf("generateOnConflictClause(update with skipped col) = %q", got)
	}
	if got := generateOnConflictClause(pkg, &MutationMethod{
		ConflictColumns: []ColumnExpr{
			{
				FieldPath: []FieldPathElement{
					{VarName: "user", Type: userType},
					{FieldName: "ID"},
				},
			},
		},
		ConflictAction: "unsupported",
	}); got != "" {
		t.Fatalf("generateOnConflictClause(unsupported action) = %q, want empty", got)
	}

	if with, err := generateWithClausesSQL(pkg, nil); err != nil || with != "" {
		t.Fatalf("generateWithClausesSQL(nil) = (%q, %v), want empty,nil", with, err)
	}

	if _, err := generateWithClauseSQL(pkg, WithClause{
		Name:       "c",
		TargetType: getTypeKey(userType),
		Filters: []*FilterExpr{
			{
				Kind: FilterComparison,
				Left: FilterOperand{IsField: true, FieldPath: []FieldPathElement{{VarName: "u"}}},
			},
		},
	}); err == nil {
		t.Fatalf("generateWithClauseSQL(filter analyze error) expected error")
	}

	if got := buildUpdateFilterQualifier("users", &MutationMethod{
		WithClauses: []WithClause{{Name: ""}},
		FromSources: []string{"   "},
	}); got == nil || got.Default != "users" {
		t.Fatalf("buildUpdateFilterQualifier(with empty names) = %+v", got)
	}

	pkgOnlyPK := &Package{Tables: map[string]*TableBinding{
		getTypeKey(userType): {
			Type:      userType,
			TypeName:  "User",
			TableName: "users",
			Fields: []FieldInfo{
				{GoName: "ID", DBName: "id", IsPrimaryKey: true},
			},
		},
	}}
	if _, err := generateUpdateSQL(pkgOnlyPK, &MutationMethod{
		Name:       "UpdatePKOnly",
		Kind:       MethodKindUpdate,
		TargetType: getTypeKey(userType),
		EntityParam: &ParamInfo{Name: "u"},
		Filters: []*FilterExpr{
			{
				Kind: FilterComparison,
				Left: FilterOperand{
					IsField: true,
					FieldPath: []FieldPathElement{
						{VarName: "u", Type: userType},
						{FieldName: "ID"},
					},
				},
				Op:    token.EQL,
				Right: FilterOperand{IsParam: true, ParamName: "id"},
			},
		},
	}); err == nil || !strings.Contains(err.Error(), "no columns to update") {
		t.Fatalf("generateUpdateSQL(entity only pk) = %v", err)
	}

	if typeForMatch(nil) != "<nil>" {
		t.Fatalf("typeForMatch(nil) should return <nil>")
	}
	if got := typeForMatch(types.Typ[types.Int]); got == "" {
		t.Fatalf("typeForMatch(int) should not be empty")
	}

	if got := formatType(&Package{}, nil); got != "" {
		t.Fatalf("formatType(nil) = %q, want empty", got)
	}
	localPkg := types.NewPackage("example.com/demo", "demo")
	localType := types.NewNamed(types.NewTypeName(token.NoPos, localPkg, "User", nil), types.NewStruct(nil, nil), nil)
	if got := formatType(&Package{TypesPkg: localPkg}, localType); got != "User" {
		t.Fatalf("formatType(local named) = %q, want User", got)
	}

	if !containsFeature([]string{" a ", "sqlx/future"}, "a") {
		t.Fatalf("containsFeature(trimmed) expected true")
	}
}
