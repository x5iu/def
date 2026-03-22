package defgen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTokenToSQLOp(t *testing.T) {
	tests := []struct {
		name string
		tok  token.Token
		want string
	}{
		{"equal", token.EQL, "="},
		{"not equal", token.NEQ, "!="},
		{"less than", token.LSS, "<"},
		{"greater than", token.GTR, ">"},
		{"less or equal", token.LEQ, "<="},
		{"greater or equal", token.GEQ, ">="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenToSQLOp(tt.tok)
			if got != tt.want {
				t.Errorf("tokenToSQLOp(%v) = %q, want %q", tt.tok, got, tt.want)
			}
		})
	}
}

func TestFormatLiteral(t *testing.T) {
	tests := []struct {
		name  string
		value string
		kind  token.Token
		want  string
	}{
		{"string literal", `"active"`, token.STRING, "'active'"},
		{"integer literal", "42", token.INT, "42"},
		{"float literal", "3.14", token.FLOAT, "3.14"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatLiteral(tt.value, tt.kind)
			if got != tt.want {
				t.Errorf("formatLiteral(%q, %v) = %q, want %q", tt.value, tt.kind, got, tt.want)
			}
		})
	}
}

func TestFormatFilterTree(t *testing.T) {
	tests := []struct {
		name   string
		filter *AnalyzedFilter
		want   string
	}{
		{
			name: "simple equal",
			filter: &AnalyzedFilter{
				Kind:       AnalyzedFilterComparison,
				ColumnName: "id",
				Operator:   "=",
				Value:      "${id}",
			},
			want: "id = ${id}",
		},
		{
			name: "string literal",
			filter: &AnalyzedFilter{
				Kind:       AnalyzedFilterComparison,
				ColumnName: "status",
				Operator:   "=",
				Value:      "'active'",
			},
			want: "status = 'active'",
		},
		{
			name: "not equal",
			filter: &AnalyzedFilter{
				Kind:       AnalyzedFilterComparison,
				ColumnName: "status",
				Operator:   "!=",
				Value:      "'deleted'",
			},
			want: "status != 'deleted'",
		},
		{
			name: "greater than",
			filter: &AnalyzedFilter{
				Kind:       AnalyzedFilterComparison,
				ColumnName: "age",
				Operator:   ">",
				Value:      "18",
			},
			want: "age > 18",
		},
		{
			name: "less than",
			filter: &AnalyzedFilter{
				Kind:       AnalyzedFilterComparison,
				ColumnName: "age",
				Operator:   "<",
				Value:      "60",
			},
			want: "age < 60",
		},
		{
			name: "subquery - foreign key",
			filter: &AnalyzedFilter{
				Kind:            AnalyzedFilterComparison,
				IsSubquery:      true,
				ForeignKeyCol:   "user_id",
				SubqueryTable:   "users",
				SubqueryColumn:  "name",
				SubqueryValue:   "${username}",
				SubqueryIDField: "id",
				Operator:        "=",
			},
			want: "user_id IN (SELECT id FROM users WHERE name = ${username})",
		},
		{
			name: "IN query",
			filter: &AnalyzedFilter{
				Kind:       AnalyzedFilterIn,
				ColumnName: "id",
				Value:      "${ids}",
			},
			want: "id IN (${ids})",
		},
		{
			name: "AND condition",
			filter: &AnalyzedFilter{
				Kind: AnalyzedFilterAnd,
				Children: []*AnalyzedFilter{
					{Kind: AnalyzedFilterComparison, ColumnName: "status", Operator: "=", Value: "'active'"},
					{Kind: AnalyzedFilterComparison, ColumnName: "user_id", Operator: "=", Value: "${id}"},
				},
			},
			want: "(status = 'active' AND user_id = ${id})",
		},
		{
			name: "OR condition",
			filter: &AnalyzedFilter{
				Kind: AnalyzedFilterOr,
				Children: []*AnalyzedFilter{
					{Kind: AnalyzedFilterComparison, ColumnName: "status", Operator: "=", Value: "'active'"},
					{Kind: AnalyzedFilterComparison, ColumnName: "status", Operator: "=", Value: "'pending'"},
				},
			},
			want: "(status = 'active' OR status = 'pending')",
		},
		{
			name: "complex nested condition",
			filter: &AnalyzedFilter{
				Kind: AnalyzedFilterOr,
				Children: []*AnalyzedFilter{
					{
						Kind: AnalyzedFilterAnd,
						Children: []*AnalyzedFilter{
							{Kind: AnalyzedFilterComparison, ColumnName: "status", Operator: "=", Value: "'active'"},
							{Kind: AnalyzedFilterComparison, ColumnName: "user_id", Operator: "=", Value: "${id}"},
						},
					},
					{Kind: AnalyzedFilterComparison, ColumnName: "status", Operator: "=", Value: "'pending'"},
				},
			},
			want: "((status = 'active' AND user_id = ${id}) OR status = 'pending')",
		},
		{
			name: "AND with IN",
			filter: &AnalyzedFilter{
				Kind: AnalyzedFilterAnd,
				Children: []*AnalyzedFilter{
					{Kind: AnalyzedFilterIn, ColumnName: "id", Value: "${ids}"},
					{Kind: AnalyzedFilterComparison, ColumnName: "status", Operator: "=", Value: "'active'"},
				},
			},
			want: "(id IN (${ids}) AND status = 'active')",
		},
		// Function call tests
		{
			name: "left side function - COUNT",
			filter: &AnalyzedFilter{
				Kind:         AnalyzedFilterComparison,
				LeftIsFunc:   true,
				LeftFuncExpr: "COUNT(id)",
				Operator:     ">",
				Value:        "0",
			},
			want: "COUNT(id) > 0",
		},
		{
			name: "left side function - SUM with param",
			filter: &AnalyzedFilter{
				Kind:         AnalyzedFilterComparison,
				LeftIsFunc:   true,
				LeftFuncExpr: "SUM(amount)",
				Operator:     ">=",
				Value:        "${minAmount}",
			},
			want: "SUM(amount) >= ${minAmount}",
		},
		{
			name: "left side function - COALESCE not equal",
			filter: &AnalyzedFilter{
				Kind:         AnalyzedFilterComparison,
				LeftIsFunc:   true,
				LeftFuncExpr: "COALESCE(name, 'Unknown')",
				Operator:     "!=",
				Value:        "'Unknown'",
			},
			want: "COALESCE(name, 'Unknown') != 'Unknown'",
		},
		{
			name: "left side function - custom function with multiple args",
			filter: &AnalyzedFilter{
				Kind:         AnalyzedFilterComparison,
				LeftIsFunc:   true,
				LeftFuncExpr: "DATE_FORMAT(created_at, '%Y-%m-%d')",
				Operator:     "=",
				Value:        "${date}",
			},
			want: "DATE_FORMAT(created_at, '%Y-%m-%d') = ${date}",
		},
		{
			name: "right side function",
			filter: &AnalyzedFilter{
				Kind:          AnalyzedFilterComparison,
				ColumnName:    "total",
				Operator:      "=",
				RightIsFunc:   true,
				RightFuncExpr: "SUM(amount)",
			},
			want: "total = SUM(amount)",
		},
		{
			name: "AND with function",
			filter: &AnalyzedFilter{
				Kind: AnalyzedFilterAnd,
				Children: []*AnalyzedFilter{
					{Kind: AnalyzedFilterComparison, LeftIsFunc: true, LeftFuncExpr: "COUNT(id)", Operator: ">", Value: "0"},
					{Kind: AnalyzedFilterComparison, ColumnName: "status", Operator: "=", Value: "'active'"},
				},
			},
			want: "(COUNT(id) > 0 AND status = 'active')",
		},
		{
			name: "OR with functions",
			filter: &AnalyzedFilter{
				Kind: AnalyzedFilterOr,
				Children: []*AnalyzedFilter{
					{Kind: AnalyzedFilterComparison, LeftIsFunc: true, LeftFuncExpr: "SUM(amount)", Operator: ">", Value: "1000"},
					{Kind: AnalyzedFilterComparison, LeftIsFunc: true, LeftFuncExpr: "COUNT(id)", Operator: ">", Value: "10"},
				},
			},
			want: "(SUM(amount) > 1000 OR COUNT(id) > 10)",
		},
		{
			name: "IS NULL",
			filter: &AnalyzedFilter{
				Kind:       AnalyzedFilterComparison,
				ColumnName: "deleted_at",
				Operator:   "IS NULL",
				IsNull:     true,
			},
			want: "deleted_at IS NULL",
		},
		{
			name: "IS NOT NULL",
			filter: &AnalyzedFilter{
				Kind:       AnalyzedFilterComparison,
				ColumnName: "name",
				Operator:   "IS NOT NULL",
				IsNull:     true,
			},
			want: "name IS NOT NULL",
		},
		{
			name: "AND with IS NULL",
			filter: &AnalyzedFilter{
				Kind: AnalyzedFilterAnd,
				Children: []*AnalyzedFilter{
					{Kind: AnalyzedFilterComparison, ColumnName: "status", Operator: "=", Value: "'active'"},
					{Kind: AnalyzedFilterComparison, ColumnName: "deleted_at", Operator: "IS NULL", IsNull: true},
				},
			},
			want: "(status = 'active' AND deleted_at IS NULL)",
		},
		{
			name: "IS NULL subquery - foreign key",
			filter: &AnalyzedFilter{
				Kind:            AnalyzedFilterComparison,
				IsSubquery:      true,
				ForeignKeyCol:   "user_id",
				SubqueryTable:   "users",
				SubqueryColumn:  "deleted_at",
				SubqueryIDField: "id",
				Operator:        "IS NULL",
				IsNull:          true,
			},
			want: "user_id IN (SELECT id FROM users WHERE deleted_at IS NULL)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatFilterTree(tt.filter)
			if got != tt.want {
				t.Errorf("formatFilterTree() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAnalyzeReturnType(t *testing.T) {
	// This test requires actual types, so we'll test the basic logic
	// For comprehensive testing, integration tests with real packages are recommended

	info := analyzeReturnType(nil)
	if info.Type != nil {
		t.Errorf("analyzeReturnType(nil) should return empty info")
	}
}

func TestFormatFilterFuncArg(t *testing.T) {
	tests := []struct {
		name string
		arg  FuncArg
		want string
	}{
		{
			name: "param reference",
			arg:  FuncArg{IsParam: true, Value: "minAmount"},
			want: "${minAmount}",
		},
		{
			name: "string literal",
			arg:  FuncArg{IsLiteral: true, Value: `"Unknown"`, Kind: token.STRING},
			want: "'Unknown'",
		},
		{
			name: "integer literal",
			arg:  FuncArg{IsLiteral: true, Value: "42", Kind: token.INT},
			want: "42",
		},
		{
			name: "float literal",
			arg:  FuncArg{IsLiteral: true, Value: "3.14", Kind: token.FLOAT},
			want: "3.14",
		},
		{
			name: "empty arg",
			arg:  FuncArg{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with nil pkg since we're not using field references
			got := formatFilterFuncArg(nil, tt.arg)
			if got != tt.want {
				t.Errorf("formatFilterFuncArg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatFuncOperand(t *testing.T) {
	tests := []struct {
		name    string
		operand FilterOperand
		want    string
	}{
		{
			name: "COUNT with no args",
			operand: FilterOperand{
				IsFunc:   true,
				FuncName: "COUNT",
				FuncArgs: []FuncArg{},
			},
			want: "COUNT()",
		},
		{
			name: "SUM with param",
			operand: FilterOperand{
				IsFunc:   true,
				FuncName: "SUM",
				FuncArgs: []FuncArg{
					{IsParam: true, Value: "amount"},
				},
			},
			want: "SUM(${amount})",
		},
		{
			name: "COALESCE with multiple args",
			operand: FilterOperand{
				IsFunc:   true,
				FuncName: "COALESCE",
				FuncArgs: []FuncArg{
					{IsParam: true, Value: "name"},
					{IsLiteral: true, Value: `"Unknown"`, Kind: token.STRING},
				},
			},
			want: "COALESCE(${name}, 'Unknown')",
		},
		{
			name: "custom function with mixed args",
			operand: FilterOperand{
				IsFunc:   true,
				FuncName: "IFNULL",
				FuncArgs: []FuncArg{
					{IsParam: true, Value: "value"},
					{IsLiteral: true, Value: "0", Kind: token.INT},
				},
			},
			want: "IFNULL(${value}, 0)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with nil pkg since we're not using field references
			got := formatFuncOperand(nil, tt.operand)
			if got != tt.want {
				t.Errorf("formatFuncOperand() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseSetValueExpressions(t *testing.T) {
	newNamed := func(pkgPath, pkgName, typeName string) *types.Named {
		p := types.NewPackage(pkgPath, pkgName)
		obj := types.NewTypeName(token.NoPos, p, typeName, nil)
		return types.NewNamed(obj, types.NewStruct(nil, nil), nil)
	}

	userType := newNamed("example.com/demo", "demo", "User")
	tableKey := getTypeKey(userType)
	baseTables := map[string]*TableBinding{
		tableKey: {
			Type:      userType,
			TypeName:  "User",
			TableName: "users",
			Fields: []FieldInfo{
				{GoName: "ID", DBName: "id"},
				{GoName: "Count", DBName: "count"},
				{GoName: "UpdatedAt", DBName: "updated_at"},
			},
		},
	}

	t.Run("field plus literal", func(t *testing.T) {
		userIdent := ast.NewIdent("user")
		expr := &ast.BinaryExpr{
			X:  &ast.SelectorExpr{X: userIdent, Sel: ast.NewIdent("Count")},
			Op: token.ADD,
			Y:  &ast.BasicLit{Kind: token.INT, Value: "1"},
		}
		pkg := &Package{
			TypesInfo: &types.Info{
				Uses: map[*ast.Ident]types.Object{
					userIdent: types.NewVar(token.NoPos, nil, "user", userType),
				},
			},
			Tables: baseTables,
		}

		value, err := parseSetValue(pkg, expr, map[string]*structInfo{}, nil)
		if err != nil {
			t.Fatalf("parseSetValue() error = %v", err)
		}
		if value.ExprSQL != "count + 1" {
			t.Fatalf("parseSetValue().ExprSQL = %q, want %q", value.ExprSQL, "count + 1")
		}
	})

	t.Run("field plus param", func(t *testing.T) {
		userIdent := ast.NewIdent("user")
		expr := &ast.BinaryExpr{
			X:  &ast.SelectorExpr{X: userIdent, Sel: ast.NewIdent("Count")},
			Op: token.ADD,
			Y:  ast.NewIdent("delta"),
		}
		pkg := &Package{
			TypesInfo: &types.Info{
				Uses: map[*ast.Ident]types.Object{
					userIdent: types.NewVar(token.NoPos, nil, "user", userType),
				},
			},
			Tables: baseTables,
		}

		value, err := parseSetValue(pkg, expr, map[string]*structInfo{}, []ParamInfo{
			{Name: "delta", Type: types.Typ[types.Int64]},
		})
		if err != nil {
			t.Fatalf("parseSetValue() error = %v", err)
		}
		if value.ExprSQL != "count + ${delta}" {
			t.Fatalf("parseSetValue().ExprSQL = %q, want %q", value.ExprSQL, "count + ${delta}")
		}
	})

	t.Run("direct function call", func(t *testing.T) {
		expr := &ast.CallExpr{Fun: ast.NewIdent("now")}
		value, err := parseSetValue(&Package{}, expr, map[string]*structInfo{}, nil)
		if err != nil {
			t.Fatalf("parseSetValue() error = %v", err)
		}
		if value.ExprSQL != "now()" {
			t.Fatalf("parseSetValue().ExprSQL = %q, want %q", value.ExprSQL, "now()")
		}
	})

	t.Run("generic def.Func call", func(t *testing.T) {
		defIdent := ast.NewIdent("def")
		expr := &ast.CallExpr{
			Fun: &ast.IndexExpr{
				X: &ast.SelectorExpr{
					X:   defIdent,
					Sel: ast.NewIdent("Func"),
				},
				Index: ast.NewIdent("any"),
			},
			Args: []ast.Expr{
				&ast.BasicLit{Kind: token.STRING, Value: `"now"`},
			},
		}
		pkg := &Package{
			TypesInfo: &types.Info{
				Uses: map[*ast.Ident]types.Object{
					defIdent: types.NewPkgName(token.NoPos, nil, "def", types.NewPackage(defPkgPath, "def")),
				},
			},
		}

		value, err := parseSetValue(pkg, expr, map[string]*structInfo{}, nil)
		if err != nil {
			t.Fatalf("parseSetValue() error = %v", err)
		}
		if value.ExprSQL != "now()" {
			t.Fatalf("parseSetValue().ExprSQL = %q, want %q", value.ExprSQL, "now()")
		}
	})

	t.Run("postgres.Excluded generates EXCLUDED.column", func(t *testing.T) {
		pgIdent := ast.NewIdent("postgres")
		roleIdent := ast.NewIdent("role")
		expr := &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   pgIdent,
				Sel: ast.NewIdent("Excluded"),
			},
			Args: []ast.Expr{
				&ast.SelectorExpr{
					X:   roleIdent,
					Sel: ast.NewIdent("Name"),
				},
			},
		}

		roleType := stubType("Role")
		pkg := &Package{
			TypesInfo: &types.Info{
				Uses: map[*ast.Ident]types.Object{
					pgIdent:   types.NewPkgName(token.NoPos, nil, "postgres", types.NewPackage(postgresPkgPath, "postgres")),
					roleIdent: types.NewVar(token.NoPos, nil, "role", types.NewPointer(roleType)),
				},
			},
			Tables: map[string]*TableBinding{
				"Role": {
					TypeName:  "Role",
					TableName: "roles",
					Fields: []FieldInfo{
						{GoName: "Name", DBName: "name"},
					},
				},
			},
		}

		value, err := parseSetValue(pkg, expr, map[string]*structInfo{}, nil)
		if err != nil {
			t.Fatalf("parseSetValue() error = %v", err)
		}
		if value.ExprSQL != "EXCLUDED.name" {
			t.Fatalf("parseSetValue().ExprSQL = %q, want %q", value.ExprSQL, "EXCLUDED.name")
		}
	})

	t.Run("postgres.Interval generates INTERVAL literal", func(t *testing.T) {
		pgIdent := ast.NewIdent("postgres")
		expr := &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   pgIdent,
				Sel: ast.NewIdent("Interval"),
			},
			Args: []ast.Expr{
				&ast.BasicLit{Kind: token.STRING, Value: `"10 minutes"`},
			},
		}
		pkg := &Package{
			TypesInfo: &types.Info{
				Uses: map[*ast.Ident]types.Object{
					pgIdent: types.NewPkgName(token.NoPos, nil, "postgres", types.NewPackage(postgresPkgPath, "postgres")),
				},
			},
		}

		value, err := parseSetValue(pkg, expr, map[string]*structInfo{}, nil)
		if err != nil {
			t.Fatalf("parseSetValue() error = %v", err)
		}
		if value.ExprSQL != "INTERVAL '10 minutes'" {
			t.Fatalf("parseSetValue().ExprSQL = %q, want %q", value.ExprSQL, "INTERVAL '10 minutes'")
		}
	})

	t.Run("postgres.Now plus postgres.Interval", func(t *testing.T) {
		pgIdent := ast.NewIdent("postgres")
		expr := &ast.BinaryExpr{
			X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   pgIdent,
					Sel: ast.NewIdent("Now"),
				},
			},
			Op: token.ADD,
			Y: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   pgIdent,
					Sel: ast.NewIdent("Interval"),
				},
				Args: []ast.Expr{
					&ast.BasicLit{Kind: token.STRING, Value: `"10 minutes"`},
				},
			},
		}
		pkg := &Package{
			TypesInfo: &types.Info{
				Uses: map[*ast.Ident]types.Object{
					pgIdent: types.NewPkgName(token.NoPos, nil, "postgres", types.NewPackage(postgresPkgPath, "postgres")),
				},
			},
		}

		value, err := parseSetValue(pkg, expr, map[string]*structInfo{}, nil)
		if err != nil {
			t.Fatalf("parseSetValue() error = %v", err)
		}
		if value.ExprSQL != "NOW() + INTERVAL '10 minutes'" {
			t.Fatalf("parseSetValue().ExprSQL = %q, want %q", value.ExprSQL, "NOW() + INTERVAL '10 minutes'")
		}
	})

	t.Run("preserve right-branch binary semantics", func(t *testing.T) {
		tests := []struct {
			name string
			expr ast.Expr
			want string
		}{
			{
				name: "multiply with right division",
				expr: &ast.BinaryExpr{
					X:  ast.NewIdent("a"),
					Op: token.MUL,
					Y: &ast.BinaryExpr{
						X:  ast.NewIdent("b"),
						Op: token.QUO,
						Y:  ast.NewIdent("c"),
					},
				},
				want: "${a} * (${b} / ${c})",
			},
			{
				name: "multiply with right remainder",
				expr: &ast.BinaryExpr{
					X:  ast.NewIdent("a"),
					Op: token.MUL,
					Y: &ast.BinaryExpr{
						X:  ast.NewIdent("b"),
						Op: token.REM,
						Y:  ast.NewIdent("c"),
					},
				},
				want: "${a} * (${b} % ${c})",
			},
			{
				name: "subtract with right subtraction",
				expr: &ast.BinaryExpr{
					X:  ast.NewIdent("a"),
					Op: token.SUB,
					Y: &ast.BinaryExpr{
						X:  ast.NewIdent("b"),
						Op: token.SUB,
						Y:  ast.NewIdent("c"),
					},
				},
				want: "${a} - (${b} - ${c})",
			},
		}

		params := []ParamInfo{
			{Name: "a", Type: types.Typ[types.Int64]},
			{Name: "b", Type: types.Typ[types.Int64]},
			{Name: "c", Type: types.Typ[types.Int64]},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				value, err := parseSetValue(&Package{}, tt.expr, map[string]*structInfo{}, params)
				if err != nil {
					t.Fatalf("parseSetValue() error = %v", err)
				}
				if value.ExprSQL != tt.want {
					t.Fatalf("parseSetValue().ExprSQL = %q, want %q", value.ExprSQL, tt.want)
				}
			})
		}
	})
}

func TestParseWithClause(t *testing.T) {
	retryType := stubType("SettlementRetry")
	tableKey := getTypeKey(retryType)

	defIdent := ast.NewIdent("def")
	postgresIdent := ast.NewIdent("postgres")
	retryIdent := ast.NewIdent("retry")
	limitIdent := ast.NewIdent("limit")

	defCall := func(name string, args ...ast.Expr) *ast.CallExpr {
		return &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   defIdent,
				Sel: ast.NewIdent(name),
			},
			Args: args,
		}
	}
	postgresCall := func(name string, args ...ast.Expr) *ast.CallExpr {
		return &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   postgresIdent,
				Sel: ast.NewIdent(name),
			},
			Args: args,
		}
	}
	field := func(name string) *ast.SelectorExpr {
		return &ast.SelectorExpr{
			X:   retryIdent,
			Sel: ast.NewIdent(name),
		}
	}

	withCall := defCall("With",
		&ast.BasicLit{Kind: token.STRING, Value: `"due"`},
		defCall("From", retryIdent),
		defCall("Column", field("ID")),
		defCall("Filter", &ast.BinaryExpr{
			X:  field("DoneAt"),
			Op: token.EQL,
			Y:  ast.NewIdent("nil"),
		}),
		defCall("Filter", &ast.BinaryExpr{
			X:  field("DeadAt"),
			Op: token.EQL,
			Y:  ast.NewIdent("nil"),
		}),
		defCall("Filter", &ast.BinaryExpr{
			X:  field("NextRetryAt"),
			Op: token.LEQ,
			Y:  postgresCall("Now"),
		}),
		defCall("OrderBy",
			defCall("Asc", field("NextRetryAt")),
			defCall("Asc", field("ID")),
		),
		defCall("Limit", limitIdent),
		postgresCall("ForUpdateSkipLocked"),
	)

	pkg := &Package{
		TypesInfo: &types.Info{
			Uses: map[*ast.Ident]types.Object{
				defIdent:      types.NewPkgName(token.NoPos, nil, "def", types.NewPackage(defPkgPath, "def")),
				postgresIdent: types.NewPkgName(token.NoPos, nil, "postgres", types.NewPackage(postgresPkgPath, "postgres")),
				retryIdent:    types.NewVar(token.NoPos, nil, "retry", retryType),
				limitIdent:    types.NewVar(token.NoPos, nil, "limit", types.Typ[types.Int]),
			},
		},
		Tables: map[string]*TableBinding{
			tableKey: {
				Type:      retryType,
				TypeName:  "SettlementRetry",
				TableName: "settlement_retries",
				Fields: []FieldInfo{
					{GoName: "ID", DBName: "id"},
					{GoName: "DoneAt", DBName: "done_at"},
					{GoName: "DeadAt", DBName: "dead_at"},
					{GoName: "NextRetryAt", DBName: "next_retry_at"},
				},
			},
		},
	}

	got, err := parseWithClause(pkg, withCall, map[string]*structInfo{}, []ParamInfo{
		{Name: "limit", Type: types.Typ[types.Int]},
	})
	if err != nil {
		t.Fatalf("parseWithClause() error = %v", err)
	}
	if got.Name != "due" {
		t.Fatalf("parseWithClause().Name = %q, want %q", got.Name, "due")
	}
	if got.TargetType != tableKey {
		t.Fatalf("parseWithClause().TargetType = %q, want %q", got.TargetType, tableKey)
	}
	if len(got.Columns) != 1 {
		t.Fatalf("parseWithClause().Columns = %d, want %d", len(got.Columns), 1)
	}
	if len(got.Filters) != 3 {
		t.Fatalf("parseWithClause().Filters = %d, want %d", len(got.Filters), 3)
	}
	if len(got.OrderBy) != 2 {
		t.Fatalf("parseWithClause().OrderBy = %d, want %d", len(got.OrderBy), 2)
	}
	if got.Limit == nil || !got.Limit.IsParam || got.Limit.ParamName != "limit" {
		t.Fatalf("parseWithClause().Limit = %+v, want parameter limit", got.Limit)
	}
	if !got.ForUpdateSkipLocked {
		t.Fatalf("parseWithClause().ForUpdateSkipLocked = false, want true")
	}
}

func TestParseDeleteArgs_RequiresAtLeastOneFilter(t *testing.T) {
	defIdent := ast.NewIdent("def")
	deleteCall := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   defIdent,
			Sel: ast.NewIdent("Delete"),
		},
	}

	pkg := &Package{
		TypesInfo: &types.Info{
			Uses: map[*ast.Ident]types.Object{
				defIdent: types.NewPkgName(token.NoPos, nil, "def", types.NewPackage(defPkgPath, "def")),
			},
		},
	}

	_, _, _, err := parseDeleteArgs(pkg, deleteCall, map[string]*structInfo{}, nil)
	if err == nil {
		t.Fatalf("parseDeleteArgs() expected error for missing delete filters")
	}
	if !strings.Contains(err.Error(), "def.Delete requires at least one Filter expression") {
		t.Fatalf("parseDeleteArgs() error = %v, want missing-filter error", err)
	}
}

func TestGenerateMutationSQL_UpdateFieldModeExpressions(t *testing.T) {
	newNamed := func(pkgPath, pkgName, typeName string) *types.Named {
		p := types.NewPackage(pkgPath, pkgName)
		obj := types.NewTypeName(token.NoPos, p, typeName, nil)
		return types.NewNamed(obj, types.NewStruct(nil, nil), nil)
	}

	userType := newNamed("example.com/demo", "demo", "User")
	tableKey := getTypeKey(userType)

	pkg := &Package{
		Tables: map[string]*TableBinding{
			tableKey: {
				Type:      userType,
				TypeName:  "User",
				TableName: "users",
				Fields: []FieldInfo{
					{GoName: "ID", DBName: "id", IsPrimaryKey: true},
					{GoName: "Count", DBName: "count"},
					{GoName: "UpdatedAt", DBName: "updated_at"},
				},
			},
		},
	}

	method := &MutationMethod{
		Kind:       MethodKindUpdate,
		Name:       "BumpCounter",
		TargetType: tableKey,
		Sets: []SetExpr{
			{
				FieldPath: []FieldPathElement{
					{VarName: "user", Type: userType},
					{FieldName: "Count"},
				},
				Value: SetValue{ExprSQL: "count + 1"},
			},
			{
				FieldPath: []FieldPathElement{
					{VarName: "user", Type: userType},
					{FieldName: "UpdatedAt"},
				},
				Value: SetValue{ExprSQL: "now()"},
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
				Right: FilterOperand{
					IsParam:   true,
					ParamName: "id",
				},
			},
		},
	}

	got, err := GenerateMutationSQL(pkg, method)
	if err != nil {
		t.Fatalf("GenerateMutationSQL() error = %v", err)
	}
	if !strings.Contains(got, "UPDATE users SET") {
		t.Fatalf("GenerateMutationSQL() = %q, want UPDATE clause", got)
	}
	if !strings.Contains(got, "count = count + 1") {
		t.Fatalf("GenerateMutationSQL() = %q, want arithmetic SET expression", got)
	}
	if !strings.Contains(got, "updated_at = now()") {
		t.Fatalf("GenerateMutationSQL() = %q, want function SET expression", got)
	}
	if !strings.Contains(got, "WHERE id = ${id}") {
		t.Fatalf("GenerateMutationSQL() = %q, want WHERE clause", got)
	}
}

func TestGenerateMutationSQL_UpdateWithCTEFrom(t *testing.T) {
	retryType := stubType("SettlementRetry")
	tableKey := getTypeKey(retryType)

	pkg := &Package{
		Tables: map[string]*TableBinding{
			tableKey: {
				Type:      retryType,
				TypeName:  "SettlementRetry",
				TableName: "settlement_retries",
				Fields: []FieldInfo{
					{GoName: "ID", DBName: "id", IsPrimaryKey: true},
					{GoName: "RequestID", DBName: "request_id"},
					{GoName: "Payload", DBName: "payload"},
					{GoName: "Attempts", DBName: "attempts"},
					{GoName: "NextRetryAt", DBName: "next_retry_at"},
					{GoName: "UpdatedAt", DBName: "updated_at"},
					{GoName: "DoneAt", DBName: "done_at"},
					{GoName: "DeadAt", DBName: "dead_at"},
				},
			},
		},
	}

	fieldPath := func(varName, field string) []FieldPathElement {
		return []FieldPathElement{
			{VarName: varName, Type: retryType},
			{FieldName: field},
		}
	}

	method := &MutationMethod{
		Kind:       MethodKindUpdate,
		Name:       "ClaimDue",
		TargetType: tableKey,
		Sets: []SetExpr{
			{
				FieldPath: fieldPath("retry", "Attempts"),
				Value:     SetValue{ExprSQL: "attempts + 1"},
			},
			{
				FieldPath: fieldPath("retry", "NextRetryAt"),
				Value:     SetValue{ExprSQL: "NOW() + INTERVAL '10 minutes'"},
			},
			{
				FieldPath: fieldPath("retry", "UpdatedAt"),
				Value:     SetValue{ExprSQL: "NOW()"},
			},
		},
		WithClauses: []WithClause{
			{
				Name:       "due",
				TargetType: tableKey,
				Columns: []ColumnExpr{
					{FieldPath: fieldPath("retry", "ID")},
				},
				Filters: []*FilterExpr{
					{
						Kind:  FilterComparison,
						Op:    token.EQL,
						Left:  FilterOperand{IsField: true, FieldPath: fieldPath("retry", "DoneAt")},
						Right: FilterOperand{IsNil: true},
					},
					{
						Kind:  FilterComparison,
						Op:    token.EQL,
						Left:  FilterOperand{IsField: true, FieldPath: fieldPath("retry", "DeadAt")},
						Right: FilterOperand{IsNil: true},
					},
					{
						Kind: FilterComparison,
						Op:   token.LEQ,
						Left: FilterOperand{IsField: true, FieldPath: fieldPath("retry", "NextRetryAt")},
						Right: FilterOperand{
							IsFunc:   true,
							FuncName: "NOW",
						},
					},
				},
				OrderBy: []OrderByExpr{
					{Column: ColumnExpr{FieldPath: fieldPath("retry", "NextRetryAt")}},
					{Column: ColumnExpr{FieldPath: fieldPath("retry", "ID")}},
				},
				Limit:               &PaginationExpr{IsParam: true, ParamName: "limit"},
				ForUpdateSkipLocked: true,
			},
		},
		FromSources: []string{"due"},
		Filters: []*FilterExpr{
			{
				Kind: FilterComparison,
				Op:   token.EQL,
				Left: FilterOperand{IsField: true, FieldPath: fieldPath("retry", "ID")},
				Right: FilterOperand{
					IsField:   true,
					FieldPath: fieldPath("due", "ID"),
				},
			},
		},
		ReturnType: &MutationReturnType{
			StructName: "SettlementRetry",
		},
		ReturningCols: []ColumnExpr{
			{FieldPath: fieldPath("retry", "ID")},
			{FieldPath: fieldPath("retry", "RequestID")},
			{FieldPath: fieldPath("retry", "Payload")},
			{FieldPath: fieldPath("retry", "Attempts")},
		},
	}

	got, err := GenerateMutationSQL(pkg, method)
	if err != nil {
		t.Fatalf("GenerateMutationSQL() error = %v", err)
	}

	wants := []string{
		"WITH due AS (SELECT id FROM settlement_retries",
		"done_at IS NULL",
		"dead_at IS NULL",
		"next_retry_at <= NOW()",
		"ORDER BY next_retry_at ASC, id ASC",
		"LIMIT ${limit}",
		"FOR UPDATE SKIP LOCKED",
		"UPDATE settlement_retries SET",
		"FROM due",
		"WHERE settlement_retries.id = due.id",
		"RETURNING id, request_id, payload, attempts",
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("GenerateMutationSQL() = %q, want to contain %q", got, want)
		}
	}
}

func TestGenerateMutationSQL_UpdateWithFromSourceQualifier(t *testing.T) {
	retryType := stubType("SettlementRetry")
	tableKey := getTypeKey(retryType)

	pkg := &Package{
		Tables: map[string]*TableBinding{
			tableKey: {
				Type:      retryType,
				TypeName:  "SettlementRetry",
				TableName: "settlement_retries",
				Fields: []FieldInfo{
					{GoName: "ID", DBName: "id", IsPrimaryKey: true},
					{GoName: "Attempts", DBName: "attempts"},
				},
			},
		},
	}

	fieldPath := func(varName, field string) []FieldPathElement {
		return []FieldPathElement{
			{VarName: varName, Type: retryType},
			{FieldName: field},
		}
	}

	method := &MutationMethod{
		Kind:       MethodKindUpdate,
		Name:       "ClaimDue",
		TargetType: tableKey,
		Sets: []SetExpr{
			{
				FieldPath: fieldPath("retry", "Attempts"),
				Value:     SetValue{ExprSQL: "attempts + 1"},
			},
		},
		FromSources: []string{"due"},
		Filters: []*FilterExpr{
			{
				Kind: FilterComparison,
				Op:   token.EQL,
				Left: FilterOperand{IsField: true, FieldPath: fieldPath("retry", "ID")},
				Right: FilterOperand{
					IsField:   true,
					FieldPath: fieldPath("due", "ID"),
				},
			},
		},
	}

	got, err := GenerateMutationSQL(pkg, method)
	if err != nil {
		t.Fatalf("GenerateMutationSQL() error = %v", err)
	}

	if !strings.Contains(got, "FROM due") {
		t.Fatalf("GenerateMutationSQL() = %q, want FROM due", got)
	}
	if !strings.Contains(got, "WHERE settlement_retries.id = due.id") {
		t.Fatalf("GenerateMutationSQL() = %q, want qualified FROM-source column", got)
	}
}

func TestGenerateMutationSQL(t *testing.T) {
	tests := []struct {
		name           string
		pkg            *Package
		method         *MutationMethod
		wantContains   []string // SQL should contain these substrings
		wantNotContain []string // SQL should NOT contain these substrings
		wantErr        bool
	}{
		// INSERT entity mode tests
		{
			name: "insert entity mode - all fields included",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"User": {
						TypeName:  "User",
						TableName: "users",
						Fields: []FieldInfo{
							{GoName: "ID", DBName: "id", IsPrimaryKey: true},
							{GoName: "Name", DBName: "name", IsPrimaryKey: false},
							{GoName: "Age", DBName: "age", IsPrimaryKey: false},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:        MethodKindCreate,
				Name:        "CreateUser",
				TargetType:  "User",
				EntityParam: &ParamInfo{Name: "user"},
			},
			wantContains: []string{
				"INSERT INTO users",
				"id", "name", "age",
				"${user.ID}", "${user.Name}", "${user.Age}",
			},
			wantErr: false,
		},
		{
			name: "insert entity mode - primary key included",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"User": {
						TypeName:  "User",
						TableName: "users",
						Fields: []FieldInfo{
							{GoName: "ID", DBName: "id", IsPrimaryKey: true},
							{GoName: "Name", DBName: "name", IsPrimaryKey: false},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:        MethodKindCreate,
				Name:        "CreateUser",
				TargetType:  "User",
				EntityParam: &ParamInfo{Name: "user"},
			},
			wantContains: []string{"id", "${user.ID}"},
			wantErr:      false,
		},
		{
			name: "insert entity mode - field order matches Fields slice",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"User": {
						TypeName:  "User",
						TableName: "users",
						Fields: []FieldInfo{
							{GoName: "ID", DBName: "id", IsPrimaryKey: true},
							{GoName: "Name", DBName: "name", IsPrimaryKey: false},
							{GoName: "Age", DBName: "age", IsPrimaryKey: false},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:        MethodKindCreate,
				Name:        "CreateUser",
				TargetType:  "User",
				EntityParam: &ParamInfo{Name: "user"},
			},
			wantContains: []string{"(id, name, age)", "(${user.ID}, ${user.Name}, ${user.Age})"},
			wantErr:      false,
		},
		{
			name: "insert entity mode - auto_increment field excluded",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"User": {
						TypeName:  "User",
						TableName: "users",
						Fields: []FieldInfo{
							{GoName: "ID", DBName: "id", IsPrimaryKey: true, IsAutoIncrement: true},
							{GoName: "Name", DBName: "name"},
							{GoName: "Age", DBName: "age"},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:        MethodKindCreate,
				Name:        "CreateUser",
				TargetType:  "User",
				EntityParam: &ParamInfo{Name: "user"},
			},
			wantContains:    []string{"INSERT INTO users", "(name, age)", "(${user.Name}, ${user.Age})"},
			wantNotContain: []string{"id", "${user.ID}"},
			wantErr:         false,
		},
		// UPDATE entity mode tests
		{
			name: "update entity mode - primary key excluded from SET",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"User": {
						TypeName:  "User",
						TableName: "users",
						Fields: []FieldInfo{
							{GoName: "ID", DBName: "id", IsPrimaryKey: true},
							{GoName: "Name", DBName: "name", IsPrimaryKey: false},
							{GoName: "Age", DBName: "age", IsPrimaryKey: false},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:        MethodKindUpdate,
				Name:        "UpdateUser",
				TargetType:  "User",
				EntityParam: &ParamInfo{Name: "user"},
				Filters:     testUpdateFilters(),
			},
			wantContains:   []string{"UPDATE users SET", "name = ${user.Name}", "age = ${user.Age}"},
			wantNotContain: []string{"id = ${user.ID}"},
			wantErr:        false,
		},
		{
			name: "update entity mode - all non-pk fields in SET",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"User": {
						TypeName:  "User",
						TableName: "users",
						Fields: []FieldInfo{
							{GoName: "ID", DBName: "id", IsPrimaryKey: true},
							{GoName: "Name", DBName: "name", IsPrimaryKey: false},
							{GoName: "Email", DBName: "email", IsPrimaryKey: false},
							{GoName: "Age", DBName: "age", IsPrimaryKey: false},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:        MethodKindUpdate,
				Name:        "UpdateUser",
				TargetType:  "User",
				EntityParam: &ParamInfo{Name: "user"},
				Filters:     testUpdateFilters(),
			},
			wantContains: []string{
				"name = ${user.Name}",
				"email = ${user.Email}",
				"age = ${user.Age}",
			},
			wantNotContain: []string{"id = ${user.ID}"},
			wantErr:        false,
		},
		{
			name: "update entity mode - no primary key includes all fields",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"Setting": {
						TypeName:  "Setting",
						TableName: "settings",
						Fields: []FieldInfo{
							{GoName: "Key", DBName: "key", IsPrimaryKey: false},
							{GoName: "Value", DBName: "value", IsPrimaryKey: false},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:        MethodKindUpdate,
				Name:        "UpdateSetting",
				TargetType:  "Setting",
				EntityParam: &ParamInfo{Name: "setting"},
				Filters:     testUpdateFilters(),
			},
			wantContains: []string{
				"UPDATE settings SET",
				"key = ${setting.Key}",
				"value = ${setting.Value}",
			},
			wantErr: false,
		},
		// Error handling tests
		{
			name: "unknown target type",
			pkg: &Package{
				Tables: map[string]*TableBinding{},
			},
			method: &MutationMethod{
				Kind:        MethodKindCreate,
				Name:        "CreateUnknown",
				TargetType:  "Unknown",
				EntityParam: &ParamInfo{Name: "entity"},
			},
			wantErr: true,
		},
		{
			name: "update entity mode with only primary key fields",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"OnlyPK": {
						TypeName:  "OnlyPK",
						TableName: "only_pk",
						Fields: []FieldInfo{
							{GoName: "ID", DBName: "id", IsPrimaryKey: true},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:        MethodKindUpdate,
				Name:        "UpdateOnlyPK",
				TargetType:  "OnlyPK",
				EntityParam: &ParamInfo{Name: "entity"},
			},
			wantErr: true, // Should error because no columns to update
		},
		// INSERT with ON CONFLICT tests
		{
			name: "insert with ON CONFLICT DO NOTHING",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"Permission": {
						TypeName:  "Permission",
						TableName: "role_permissions",
						Fields: []FieldInfo{
							{GoName: "ID", DBName: "id", IsPrimaryKey: true},
							{GoName: "RoleID", DBName: "role_id"},
							{GoName: "Resource", DBName: "resource"},
							{GoName: "Action", DBName: "action"},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:       MethodKindCreate,
				Name:       "UpsertPermission",
				TargetType: "Permission",
				Sets: []SetExpr{
					{FieldPath: []FieldPathElement{{VarName: "perm", Type: stubType("Permission")}, {FieldName: "RoleID"}}, Value: SetValue{IsParam: true, ParamName: "roleID"}},
					{FieldPath: []FieldPathElement{{VarName: "perm", Type: stubType("Permission")}, {FieldName: "Resource"}}, Value: SetValue{IsParam: true, ParamName: "resource"}},
					{FieldPath: []FieldPathElement{{VarName: "perm", Type: stubType("Permission")}, {FieldName: "Action"}}, Value: SetValue{IsParam: true, ParamName: "action"}},
				},
				ConflictColumns: []ColumnExpr{
					{FieldPath: []FieldPathElement{{VarName: "perm", Type: stubType("Permission")}, {FieldName: "RoleID"}}},
					{FieldPath: []FieldPathElement{{VarName: "perm", Type: stubType("Permission")}, {FieldName: "Resource"}}},
					{FieldPath: []FieldPathElement{{VarName: "perm", Type: stubType("Permission")}, {FieldName: "Action"}}},
				},
				ConflictAction: "nothing",
			},
			wantContains: []string{
				"INSERT INTO role_permissions",
				"ON CONFLICT (role_id, resource, action) DO NOTHING",
			},
			wantNotContain: []string{"RETURNING"},
		},
		{
			name: "insert with ON CONFLICT DO UPDATE SET",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"Role": {
						TypeName:  "Role",
						TableName: "roles",
						Fields: []FieldInfo{
							{GoName: "ID", DBName: "id", IsPrimaryKey: true},
							{GoName: "Name", DBName: "name"},
							{GoName: "CreatedAt", DBName: "created_at"},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:       MethodKindCreate,
				Name:       "UpsertRole",
				TargetType: "Role",
				Sets: []SetExpr{
					{FieldPath: []FieldPathElement{{VarName: "role", Type: stubType("Role")}, {FieldName: "Name"}}, Value: SetValue{IsParam: true, ParamName: "name"}},
					{FieldPath: []FieldPathElement{{VarName: "role", Type: stubType("Role")}, {FieldName: "CreatedAt"}}, Value: SetValue{ExprSQL: "now()"}},
				},
				ConflictColumns: []ColumnExpr{
					{FieldPath: []FieldPathElement{{VarName: "role", Type: stubType("Role")}, {FieldName: "Name"}}},
				},
				ConflictAction: "update",
				ConflictSets: []SetExpr{
					{FieldPath: []FieldPathElement{{VarName: "role", Type: stubType("Role")}, {FieldName: "Name"}}, Value: SetValue{ExprSQL: "EXCLUDED.name"}},
				},
			},
			wantContains: []string{
				"INSERT INTO roles",
				"ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name",
			},
			wantNotContain: []string{"RETURNING"},
		},
		{
			name: "insert with ON CONFLICT DO UPDATE + RETURNING",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"Role": {
						TypeName:  "Role",
						TableName: "roles",
						Fields: []FieldInfo{
							{GoName: "ID", DBName: "id", IsPrimaryKey: true},
							{GoName: "Name", DBName: "name"},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:       MethodKindCreate,
				Name:       "UpsertRoleReturning",
				TargetType: "Role",
				Sets: []SetExpr{
					{FieldPath: []FieldPathElement{{VarName: "role", Type: stubType("Role")}, {FieldName: "Name"}}, Value: SetValue{IsParam: true, ParamName: "name"}},
				},
				ConflictColumns: []ColumnExpr{
					{FieldPath: []FieldPathElement{{VarName: "role", Type: stubType("Role")}, {FieldName: "Name"}}},
				},
				ConflictAction: "update",
				ConflictSets: []SetExpr{
					{FieldPath: []FieldPathElement{{VarName: "role", Type: stubType("Role")}, {FieldName: "Name"}}, Value: SetValue{ExprSQL: "EXCLUDED.name"}},
				},
				ReturnType: &MutationReturnType{
					StructName: "Role",
				},
			},
			wantContains: []string{
				"INSERT INTO roles",
				"ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name",
				"RETURNING *",
			},
		},
		{
			name: "insert with ON CONFLICT DO NOTHING + RETURNING id",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"Role": {
						TypeName:  "Role",
						TableName: "roles",
						Fields: []FieldInfo{
							{GoName: "ID", DBName: "id", IsPrimaryKey: true},
							{GoName: "Name", DBName: "name"},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:       MethodKindCreate,
				Name:       "UpsertRoleReturnID",
				TargetType: "Role",
				Sets: []SetExpr{
					{FieldPath: []FieldPathElement{{VarName: "role", Type: stubType("Role")}, {FieldName: "Name"}}, Value: SetValue{IsParam: true, ParamName: "name"}},
				},
				ConflictColumns: []ColumnExpr{
					{FieldPath: []FieldPathElement{{VarName: "role", Type: stubType("Role")}, {FieldName: "Name"}}},
				},
				ConflictAction: "nothing",
				ReturnType: &MutationReturnType{
					IsScalar:   true,
					StructName: "int64",
				},
				ReturningCols: []ColumnExpr{
					{FieldPath: []FieldPathElement{{VarName: "role", Type: stubType("Role")}, {FieldName: "ID"}}},
				},
			},
			wantContains: []string{
				"INSERT INTO roles",
				"ON CONFLICT (name) DO NOTHING",
				"RETURNING id",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerateMutationSQL(tt.pkg, tt.method)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateMutationSQL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Check that SQL contains expected substrings
			for _, want := range tt.wantContains {
				if !contains(got, want) {
					t.Errorf("GenerateMutationSQL() = %q, want to contain %q", got, want)
				}
			}

			// Check that SQL does NOT contain unwanted substrings
			for _, notWant := range tt.wantNotContain {
				if contains(got, notWant) {
					t.Errorf("GenerateMutationSQL() = %q, should NOT contain %q", got, notWant)
				}
			}
		})
	}
}

// contains checks if s contains substr.
func contains(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// stubType creates a minimal *types.Named for test FieldPathElement.Type.
// It uses a nil package so getTypeKey() resolves to the plain type name.
func stubType(typeName string) *types.Named {
	obj := types.NewTypeName(token.NoPos, nil, typeName, nil)
	return types.NewNamed(obj, types.NewStruct(nil, nil), nil)
}

func testUpdateFilters() []*FilterExpr {
	return []*FilterExpr{
		{
			Kind: FilterComparison,
			Op:   token.GTR,
			Left: FilterOperand{
				IsFunc:   true,
				FuncName: "COUNT",
				FuncArgs: []FuncArg{
					{IsLiteral: true, Value: "1", Kind: token.INT},
				},
			},
			Right: FilterOperand{IsLiteral: true, LiteralValue: "0", LiteralKind: token.INT},
		},
	}
}

func testDeleteFilters() []*FilterExpr {
	userType := stubType("User")
	return []*FilterExpr{
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
			Right: FilterOperand{
				IsParam:   true,
				ParamName: "id",
			},
		},
	}
}

func TestGenerateCallbackMethod_BelongsToRefTypeName(t *testing.T) {
	// Scenario: struct field name "Author" differs from type name "User"
	// e.g., Author *User `db:"-" foreign_key:"author_id"`
	cb := &CallbackMethod{
		StructName:     "Project",
		StructTypeName: "*Project",
		IDField: &FieldInfo{
			GoName: "ID",
			DBName: "id",
		},
		Fields: []CallbackField{
			{
				FieldName:    "Author",
				RefTypeName:  "User",
				MethodName:   "getUserByID",
				KeyFieldName: "AuthorID",
				IsSlice:      false,
				CacheKey:     "author_id",
			},
		},
	}

	got := generateCallbackMethod(cb, "Querier")

	// Should use *User (actual type) not *Author (field name) in requireCache/setCache
	if strings.Contains(got, "*Author") {
		t.Errorf("generateCallbackMethod() should use type name 'User', not field name 'Author'.\nGot:\n%s", got)
	}
	if !strings.Contains(got, "requireCache[*User]") {
		t.Errorf("generateCallbackMethod() should contain requireCache[*User].\nGot:\n%s", got)
	}
	// Field access should still use the field name "Author"
	if !strings.Contains(got, "p.Author") {
		t.Errorf("generateCallbackMethod() should access field as p.Author.\nGot:\n%s", got)
	}
}

func TestBuildDefcFeatures(t *testing.T) {
	tests := []struct {
		name        string
		hasCallback bool
		extra       string
		want        string
	}{
		{"callback only", true, "", "sqlx/callback"},
		{"callback with extra", true, "sqlx/rebind,sqlx/in", "sqlx/callback,sqlx/rebind,sqlx/in"},
		{"extra only", false, "sqlx/rebind,sqlx/in", "sqlx/rebind,sqlx/in"},
		{"nothing", false, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDefcFeatures(tt.hasCallback, tt.extra)
			if got != tt.want {
				t.Errorf("buildDefcFeatures(%v, %q) = %q, want %q", tt.hasCallback, tt.extra, got, tt.want)
			}
		})
	}
}

func TestFindMatchingInterface_UsesExactSignature(t *testing.T) {
	errType := types.Universe.Lookup("error").Type()

	pkg := &Package{
		Methods: []*QueryMethod{
			{
				Name:        "GetName",
				ParamTypes:  []types.Type{types.Typ[types.Int64]},
				ResultTypes: []types.Type{types.Typ[types.String], errType},
			},
		},
		Interfaces: map[string]*InterfaceInfo{
			"BadByNameOnly": {
				Name: "BadByNameOnly",
				Methods: []InterfaceMethod{
					{
						Name:        "GetName",
						ParamTypes:  []types.Type{types.Typ[types.String]},
						ResultTypes: []types.Type{types.Typ[types.String], errType},
					},
				},
			},
			"Good": {
				Name: "Good",
				Methods: []InterfaceMethod{
					{
						Name:        "GetName",
						ParamTypes:  []types.Type{types.Typ[types.Int64]},
						ResultTypes: []types.Type{types.Typ[types.String], errType},
					},
				},
			},
			"GoodSuperset": {
				Name: "GoodSuperset",
				Methods: []InterfaceMethod{
					{
						Name:        "GetName",
						ParamTypes:  []types.Type{types.Typ[types.Int64]},
						ResultTypes: []types.Type{types.Typ[types.String], errType},
					},
					{
						Name:        "Extra",
						ParamTypes:  []types.Type{types.Typ[types.Int]},
						ResultTypes: []types.Type{errType},
					},
				},
			},
		},
	}

	iface, err := findMatchingInterface(pkg)
	if err != nil {
		t.Fatalf("findMatchingInterface() error = %v", err)
	}
	if iface == nil {
		t.Fatalf("findMatchingInterface() returned nil")
	}
	if iface.Name != "Good" {
		t.Fatalf("findMatchingInterface() = %s, want Good", iface.Name)
	}
}

func TestFindMatchingInterface_AmbiguousExactMatches(t *testing.T) {
	errType := types.Universe.Lookup("error").Type()

	pkg := &Package{
		Methods: []*QueryMethod{
			{
				Name:        "GetName",
				ParamTypes:  []types.Type{types.Typ[types.Int64]},
				ResultTypes: []types.Type{types.Typ[types.String], errType},
			},
		},
		Interfaces: map[string]*InterfaceInfo{
			"First": {
				Name: "First",
				Methods: []InterfaceMethod{
					{
						Name:        "GetName",
						ParamTypes:  []types.Type{types.Typ[types.Int64]},
						ResultTypes: []types.Type{types.Typ[types.String], errType},
					},
				},
			},
			"Second": {
				Name: "Second",
				Methods: []InterfaceMethod{
					{
						Name:        "GetName",
						ParamTypes:  []types.Type{types.Typ[types.Int64]},
						ResultTypes: []types.Type{types.Typ[types.String], errType},
					},
				},
			},
		},
	}

	iface, err := findMatchingInterface(pkg)
	if err == nil {
		t.Fatalf("findMatchingInterface() error = nil, iface = %+v, want ambiguous error", iface)
	}
	if !strings.Contains(err.Error(), "multiple exact matching interfaces") {
		t.Fatalf("findMatchingInterface() error = %v, want ambiguous exact-match error", err)
	}
}

func TestLookupTableByType_CrossPackageBindTable(t *testing.T) {
	newNamed := func(pkgPath, pkgName, typeName string) *types.Named {
		p := types.NewPackage(pkgPath, pkgName)
		obj := types.NewTypeName(token.NoPos, p, typeName, nil)
		return types.NewNamed(obj, types.NewStruct(nil, nil), nil)
	}

	userA := newNamed("example.com/a", "a", "User")
	userB := newNamed("example.com/b", "b", "User")

	pkg := &Package{
		Tables: map[string]*TableBinding{
			getTypeKey(userA): {TypeName: "User", TableName: "users_a"},
			getTypeKey(userB): {TypeName: "User", TableName: "users_b"},
		},
	}

	gotA, err := lookupTableByType(pkg, userA)
	if err != nil {
		t.Fatalf("lookupTableByType(userA) error = %v", err)
	}
	if gotA.TableName != "users_a" {
		t.Fatalf("lookupTableByType(userA) = %s, want users_a", gotA.TableName)
	}

	gotB, err := lookupTableByType(pkg, userB)
	if err != nil {
		t.Fatalf("lookupTableByType(userB) error = %v", err)
	}
	if gotB.TableName != "users_b" {
		t.Fatalf("lookupTableByType(userB) = %s, want users_b", gotB.TableName)
	}

}

func TestSplitConditions_IgnoreQuotedAndOr(t *testing.T) {
	parts := splitConditions("name = 'A AND B' AND status = 'X OR Y' OR age > 1")
	if len(parts) != 3 {
		t.Fatalf("splitConditions() len = %d, want 3 (%+v)", len(parts), parts)
	}
	if parts[0].condition != "name = 'A AND B'" {
		t.Fatalf("parts[0].condition = %q, want %q", parts[0].condition, "name = 'A AND B'")
	}
	if parts[1].connector != "AND" || strings.TrimSpace(parts[1].condition) != "status = 'X OR Y'" {
		t.Fatalf("parts[1] = %+v, want connector AND with quoted OR preserved", parts[1])
	}
	if parts[2].connector != "OR" || strings.TrimSpace(parts[2].condition) != "age > 1" {
		t.Fatalf("parts[2] = %+v, want connector OR + age condition", parts[2])
	}
}

func TestGenerateMutationSQL_UpdateWithoutFilterReturnsError(t *testing.T) {
	pkg := &Package{
		Tables: map[string]*TableBinding{
			"User": {
				TypeName:  "User",
				TableName: "users",
				Fields: []FieldInfo{
					{GoName: "ID", DBName: "id", IsPrimaryKey: true},
					{GoName: "Name", DBName: "name"},
				},
			},
		},
	}
	method := &MutationMethod{
		Kind:        MethodKindUpdate,
		Name:        "UpdateUser",
		TargetType:  "User",
		EntityParam: &ParamInfo{Name: "user"},
	}

	_, err := GenerateMutationSQL(pkg, method)
	if err == nil {
		t.Fatalf("GenerateMutationSQL() expected error for missing update filters")
	}
	if !strings.Contains(err.Error(), "requires at least one Filter expression") {
		t.Fatalf("GenerateMutationSQL() error = %v, want missing-filter error", err)
	}
}

func TestGenerateMutationSQL_DeleteWithoutFilterReturnsError(t *testing.T) {
	pkg := &Package{
		Tables: map[string]*TableBinding{
			"User": {
				TypeName:  "User",
				TableName: "users",
				Fields: []FieldInfo{
					{GoName: "ID", DBName: "id", IsPrimaryKey: true},
					{GoName: "Name", DBName: "name"},
				},
			},
		},
	}
	method := &MutationMethod{
		Kind:       MethodKindDelete,
		Name:       "DeleteUser",
		TargetType: "User",
	}

	_, err := GenerateMutationSQL(pkg, method)
	if err == nil {
		t.Fatalf("GenerateMutationSQL() expected error for missing delete filters")
	}
	if !strings.Contains(err.Error(), "requires at least one Filter expression") {
		t.Fatalf("GenerateMutationSQL() error = %v, want missing-filter error", err)
	}
}

func TestAnalyzeFilter_InvalidPathReturnsError(t *testing.T) {
	filter := &FilterExpr{
		Kind: FilterComparison,
		Op:   token.EQL,
		Left: FilterOperand{
			IsField:   true,
			FieldPath: []FieldPathElement{{VarName: "u"}},
		},
		Right: FilterOperand{
			IsLiteral:    true,
			LiteralValue: "1",
			LiteralKind:  token.INT,
		},
	}

	_, err := AnalyzeFilter(&Package{Tables: map[string]*TableBinding{}}, filter)
	if err == nil {
		t.Fatalf("AnalyzeFilter() expected error for invalid field path")
	}
}

func TestGenerateCallbackMethod_HasManyCustomFieldName(t *testing.T) {
	// Scenario: has-many field name "Endpoints" differs from alias type "ModelEndpointRows"
	// e.g., Endpoints []*ModelEndpointRow `db:"-"` with inverse:"Endpoints" on the FK side
	cb := &CallbackMethod{
		StructName:     "Model",
		StructTypeName: "*Model",
		IDField: &FieldInfo{
			GoName: "ID",
			DBName: "id",
		},
		Fields: []CallbackField{
			{
				FieldName:     "Endpoints",
				AliasTypeName: "ModelEndpointRows",
				MethodName:    "getModelEndpointRowsByModelID",
				KeyFieldName:  "ID",
				IsSlice:       true,
				CacheKey:      "model_id",
				SliceType:     "[]*ModelEndpointRow",
				FieldIsAlias:  false,
			},
		},
	}

	got := generateCallbackMethod(cb, "Store")

	// Should use alias type name "ModelEndpointRows" for requireCache/setCache, not field name "Endpoints"
	if !strings.Contains(got, "requireCache[ModelEndpointRows]") {
		t.Errorf("should use alias type name in requireCache, not field name.\nGot:\n%s", got)
	}
	if strings.Contains(got, "requireCache[Endpoints]") {
		t.Errorf("should NOT use field name 'Endpoints' as type in requireCache.\nGot:\n%s", got)
	}
	// Field access should use the actual field name "Endpoints"
	if !strings.Contains(got, "m.Endpoints") {
		t.Errorf("should access field as m.Endpoints.\nGot:\n%s", got)
	}
	// setCache should use alias type for conversion: ModelEndpointRows(m.Endpoints)
	if !strings.Contains(got, "ModelEndpointRows(m.Endpoints)") {
		t.Errorf("should convert via alias type: ModelEndpointRows(m.Endpoints).\nGot:\n%s", got)
	}
}

func TestGenerateCallbackMethod_PrecacheSelfByHasManyForeignKey(t *testing.T) {
	cb := &CallbackMethod{
		StructName:     "Model",
		StructTypeName: "*Model",
		IDField: &FieldInfo{
			GoName: "ID",
			DBName: "id",
		},
		Fields: []CallbackField{
			{
				FieldName:     "Endpoints",
				AliasTypeName: "ModelEndpointRows",
				MethodName:    "getModelEndpointRowsByModelID",
				KeyFieldName:  "ID",
				IsSlice:       true,
				CacheKey:      "model_id",
				SliceType:     "[]*ModelEndpointRow",
				FieldIsAlias:  false,
			},
			{
				FieldName:    "Owner",
				RefTypeName:  "User",
				MethodName:   "getUserByID",
				KeyFieldName: "OwnerID",
				IsSlice:      false,
				CacheKey:     "owner_id",
			},
		},
	}

	got := generateCallbackMethod(cb, "Store")

	if !strings.Contains(got, "setCache(ctx, fmt.Sprintf(\"id:%v\", m.ID), m)") {
		t.Errorf("should pre-cache self by primary key.\nGot:\n%s", got)
	}
	if !strings.Contains(got, "setCache(ctx, fmt.Sprintf(\"model_id:%v\", m.ID), m)") {
		t.Errorf("should also pre-cache self by has-many foreign key.\nGot:\n%s", got)
	}
	if strings.Contains(got, "setCache(ctx, fmt.Sprintf(\"owner_id:%v\", m.OwnerID), m)") {
		t.Errorf("should not pre-cache self for belongs-to foreign keys.\nGot:\n%s", got)
	}
}

func TestCallbackWithoutCache_ReturnsError(t *testing.T) {
	pkg := &Package{
		PkgName: "testpkg",
		CallbackMethods: []*CallbackMethod{
			{
				StructName:     "Project",
				StructTypeName: "*Project",
				IDField: &FieldInfo{
					GoName: "ID",
					DBName: "id",
				},
				Fields: []CallbackField{
					{
						FieldName:    "Author",
						RefTypeName:  "User",
						MethodName:   "getUserByID",
						KeyFieldName: "AuthorID",
						CacheKey:     "author_id",
					},
				},
			},
		},
	}

	gotBytes, err := generateCode(pkg, &GenerateOptions{InterfaceName: "Store"})
	if err != nil {
		t.Fatalf("generateCode() error = %v", err)
	}

	got := string(gotBytes)

	if !strings.Contains(got, "var ErrCallbackCacheRequired = errors.New(\"callback requires WithCache context; see WithCache()\")") {
		t.Fatalf("generated code should define ErrCallbackCacheRequired sentinel error.\nGot:\n%s", got)
	}
	if !strings.Contains(got, "if cached, ok, err := requireCache[*User]") {
		t.Fatalf("generated Callback should call requireCache and handle missing cache.\nGot:\n%s", got)
	}
	if !strings.Contains(got, "if _, ok := ctx.Value(callbackCacheKey{}).(callbackCache); !ok {") {
		t.Fatalf("generated Callback should fail fast at method start when cache context is missing.\nGot:\n%s", got)
	}
	if !strings.Contains(got, "return ErrCallbackCacheRequired") {
		t.Fatalf("generated Callback should return ErrCallbackCacheRequired when cache context is missing.\nGot:\n%s", got)
	}
	if !strings.Contains(got, "return v, false, ErrCallbackCacheRequired") {
		t.Fatalf("generated requireCache should fail fast when WithCache context is missing.\nGot:\n%s", got)
	}
	if !strings.Contains(got, "\"errors\"") {
		t.Fatalf("generated imports should include errors for sentinel error.\nGot:\n%s", got)
	}
}

func TestGenerateReturningClause(t *testing.T) {
	pkg := &Package{
		Tables: map[string]*TableBinding{
			"User": {
				TypeName:  "User",
				TableName: "users",
				Fields: []FieldInfo{
					{GoName: "ID", DBName: "id", IsPrimaryKey: true},
					{GoName: "Name", DBName: "name", IsPrimaryKey: false},
					{GoName: "Age", DBName: "age", IsPrimaryKey: false},
				},
			},
		},
	}

	tests := []struct {
		name   string
		method *MutationMethod
		want   string
	}{
		{
			name: "no return type - no RETURNING",
			method: &MutationMethod{
				Kind:        MethodKindCreate,
				Name:        "CreateUser",
				TargetType:  "User",
				EntityParam: &ParamInfo{Name: "user"},
				ReturnType:  nil, // sql.Result
			},
			want: "",
		},
		{
			name: "has return type without explicit columns - RETURNING *",
			method: &MutationMethod{
				Kind:        MethodKindCreate,
				Name:        "CreateUser",
				TargetType:  "User",
				EntityParam: &ParamInfo{Name: "user"},
				ReturnType: &MutationReturnType{
					StructName: "User",
				},
				ReturningCols: nil, // No explicit columns
			},
			want: " RETURNING *",
		},
		{
			name: "has return type with empty explicit columns - RETURNING *",
			method: &MutationMethod{
				Kind:        MethodKindCreate,
				Name:        "CreateUser",
				TargetType:  "User",
				EntityParam: &ParamInfo{Name: "user"},
				ReturnType: &MutationReturnType{
					StructName: "User",
				},
				ReturningCols: []ColumnExpr{}, // Empty slice
			},
			want: " RETURNING *",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateReturningClause(pkg, tt.method)
			if got != tt.want {
				t.Errorf("generateReturningClause() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAnalyzeMutationReturnType(t *testing.T) {
	tests := []struct {
		name       string
		returnType *MutationReturnType
		wantNil    bool
	}{
		{
			name:       "nil type returns nil",
			returnType: analyzeMutationReturnType(nil),
			wantNil:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantNil && tt.returnType != nil {
				t.Errorf("analyzeMutationReturnType() = %v, want nil", tt.returnType)
			}
		})
	}
}

func TestGenerateMutationSQLWithReturning(t *testing.T) {
	tests := []struct {
		name           string
		pkg            *Package
		method         *MutationMethod
		wantContains   []string
		wantNotContain []string
		wantErr        bool
	}{
		// INSERT with RETURNING
		{
			name: "insert with RETURNING *",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"User": {
						TypeName:  "User",
						TableName: "users",
						Fields: []FieldInfo{
							{GoName: "ID", DBName: "id", IsPrimaryKey: true},
							{GoName: "Name", DBName: "name", IsPrimaryKey: false},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:        MethodKindCreate,
				Name:        "CreateUser",
				TargetType:  "User",
				EntityParam: &ParamInfo{Name: "user"},
				ReturnType: &MutationReturnType{
					StructName: "User",
				},
			},
			wantContains: []string{
				"INSERT INTO users",
				"RETURNING *",
			},
			wantErr: false,
		},
		{
			name: "insert without RETURNING (sql.Result)",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"User": {
						TypeName:  "User",
						TableName: "users",
						Fields: []FieldInfo{
							{GoName: "ID", DBName: "id", IsPrimaryKey: true},
							{GoName: "Name", DBName: "name", IsPrimaryKey: false},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:        MethodKindCreate,
				Name:        "CreateUser",
				TargetType:  "User",
				EntityParam: &ParamInfo{Name: "user"},
				ReturnType:  nil, // sql.Result
			},
			wantContains:   []string{"INSERT INTO users"},
			wantNotContain: []string{"RETURNING"},
			wantErr:        false,
		},
		// UPDATE with RETURNING
		{
			name: "update with RETURNING *",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"User": {
						TypeName:  "User",
						TableName: "users",
						Fields: []FieldInfo{
							{GoName: "ID", DBName: "id", IsPrimaryKey: true},
							{GoName: "Name", DBName: "name", IsPrimaryKey: false},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:        MethodKindUpdate,
				Name:        "UpdateUser",
				TargetType:  "User",
				EntityParam: &ParamInfo{Name: "user"},
				Filters:     testUpdateFilters(),
				ReturnType: &MutationReturnType{
					StructName: "User",
				},
			},
			wantContains: []string{
				"UPDATE users SET",
				"RETURNING *",
			},
			wantErr: false,
		},
		{
			name: "update without RETURNING",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"User": {
						TypeName:  "User",
						TableName: "users",
						Fields: []FieldInfo{
							{GoName: "ID", DBName: "id", IsPrimaryKey: true},
							{GoName: "Name", DBName: "name", IsPrimaryKey: false},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:        MethodKindUpdate,
				Name:        "UpdateUser",
				TargetType:  "User",
				EntityParam: &ParamInfo{Name: "user"},
				Filters:     testUpdateFilters(),
				ReturnType:  nil,
			},
			wantContains:   []string{"UPDATE users SET"},
			wantNotContain: []string{"RETURNING"},
			wantErr:        false,
		},
		// DELETE with RETURNING
		{
			name: "delete with RETURNING *",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"User": {
						TypeName:  "User",
						TableName: "users",
						Fields: []FieldInfo{
							{GoName: "ID", DBName: "id", IsPrimaryKey: true},
							{GoName: "Name", DBName: "name", IsPrimaryKey: false},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:       MethodKindDelete,
				Name:       "DeleteUser",
				TargetType: "User",
				Filters:    testDeleteFilters(),
				ReturnType: &MutationReturnType{
					StructName: "User",
				},
			},
			wantContains: []string{
				"DELETE FROM users",
				"RETURNING *",
			},
			wantErr: false,
		},
		{
			name: "delete without RETURNING",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"User": {
						TypeName:  "User",
						TableName: "users",
						Fields: []FieldInfo{
							{GoName: "ID", DBName: "id", IsPrimaryKey: true},
							{GoName: "Name", DBName: "name", IsPrimaryKey: false},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:       MethodKindDelete,
				Name:       "DeleteUser",
				TargetType: "User",
				Filters:    testDeleteFilters(),
				ReturnType: nil,
			},
			wantContains:   []string{"DELETE FROM users"},
			wantNotContain: []string{"RETURNING"},
			wantErr:        false,
		},
		// INSERT with scalar return type (RETURNING *)
		{
			name: "insert with scalar return type",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"User": {
						TypeName:  "User",
						TableName: "users",
						Fields: []FieldInfo{
							{GoName: "ID", DBName: "id", IsPrimaryKey: true},
							{GoName: "Name", DBName: "name", IsPrimaryKey: false},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:        MethodKindCreate,
				Name:        "CreateUser",
				TargetType:  "User",
				EntityParam: &ParamInfo{Name: "user"},
				ReturnType: &MutationReturnType{
					IsScalar:   true,
					StructName: "int64",
				},
				// No explicit ReturningCols, will use RETURNING *
			},
			wantContains: []string{
				"INSERT INTO users",
				"RETURNING *",
			},
			wantErr: false,
		},
		// INSERT with slice return type
		{
			name: "insert with slice return type",
			pkg: &Package{
				Tables: map[string]*TableBinding{
					"User": {
						TypeName:  "User",
						TableName: "users",
						Fields: []FieldInfo{
							{GoName: "ID", DBName: "id", IsPrimaryKey: true},
							{GoName: "Name", DBName: "name", IsPrimaryKey: false},
						},
					},
				},
			},
			method: &MutationMethod{
				Kind:        MethodKindCreate,
				Name:        "CreateUsers",
				TargetType:  "User",
				EntityParam: &ParamInfo{Name: "user"},
				ReturnType: &MutationReturnType{
					IsSlice:    true,
					StructName: "User",
				},
			},
			wantContains: []string{
				"INSERT INTO users",
				"RETURNING *",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerateMutationSQL(tt.pkg, tt.method)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateMutationSQL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			for _, want := range tt.wantContains {
				if !contains(got, want) {
					t.Errorf("GenerateMutationSQL() = %q, want to contain %q", got, want)
				}
			}

			for _, notWant := range tt.wantNotContain {
				if contains(got, notWant) {
					t.Errorf("GenerateMutationSQL() = %q, should NOT contain %q", got, notWant)
				}
			}
		})
	}
}

func TestInvokeDefcConsistency(t *testing.T) {
	// Minimal intermediate Go file (simulates def generate output)
	const intermediateSource = `package testpkg

import "context"

type Store interface {
	// GetUserByID query constbind
	/* SELECT *
	FROM users
	WHERE id = ${iD} */
	GetUserByID(ctx context.Context, iD int64) (*User, error)
}
`

	// Write intermediate file
	dir := t.TempDir()
	filePath := filepath.Join(dir, "store.go")
	if err := os.WriteFile(filePath, []byte(intermediateSource), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	interfaceName := "Store"
	featureList := "sqlx/rebind,sqlx/in"

	// Path A: run defc CLI command
	defcCmd := strings.TrimSpace(os.Getenv("DEFC_TEST_CMD"))
	if defcCmd == "" {
		defcCmd = "go run -mod=mod github.com/x5iu/defc@latest"
	}
	if err := runDefcGenerate(defcCmd, dir, interfaceName, featureList, "store.go", "store_impl_cli.go"); err != nil {
		t.Fatalf("defc CLI generate failed: %v", err)
	}
	resultA, err := os.ReadFile(filepath.Join(dir, "store_impl_cli.go"))
	if err != nil {
		t.Fatalf("failed to read defc CLI output: %v", err)
	}

	// Path B: invokeDefc (code generation path used by --defc-generate)
	pkg := &Package{
		CallbackMethods: nil, // no callbacks in this test
	}
	opts := &GenerateOptions{
		InterfaceName: interfaceName,
		DefcFeatures:  featureList,
		DefcGenerate:  true,
	}
	if err := invokeDefc(filePath, []byte(intermediateSource), pkg, opts); err != nil {
		t.Fatalf("invokeDefc failed: %v", err)
	}

	resultB, err := os.ReadFile(filepath.Join(dir, "store_impl.go"))
	if err != nil {
		t.Fatalf("failed to read invokeDefc output: %v", err)
	}

	// Compare
	if !bytes.Equal(resultA, resultB) {
		t.Errorf("invokeDefc output differs from defc CLI output.\n--- defc CLI ---\n%s\n--- invokeDefc ---\n%s",
			string(resultA), string(resultB))
	}
}

func runDefcGenerate(defcCmd, dir, interfaceName, features, inputFile, outputFile string) error {
	cmdParts := strings.Fields(defcCmd)
	if len(cmdParts) == 0 {
		return fmt.Errorf("empty defc command")
	}

	args := append([]string{}, cmdParts[1:]...)
	args = append(args, "generate")
	if features != "" {
		args = append(args, "--features", features)
	}
	args = append(args, "-T", interfaceName, "-o", outputFile, inputFile)

	cmd := exec.Command(cmdParts[0], args...)
	cmd.Dir = dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run command %q failed: %w\nstdout:\n%s\nstderr:\n%s",
			strings.Join(append([]string{cmdParts[0]}, args...), " "),
			err,
			stdout.String(),
			stderr.String())
	}

	return nil
}

func TestFormatSQL_OnConflict(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{
			name: "INSERT with ON CONFLICT DO NOTHING",
			sql:  "INSERT INTO role_permissions (role_id, resource, action) VALUES (${roleID}, ${resource}, ${action}) ON CONFLICT (role_id, resource, action) DO NOTHING",
			want: "INSERT INTO role_permissions (\n    role_id,\n    resource,\n    action\n) VALUES (\n    ${roleID},\n    ${resource},\n    ${action}\n)\nON CONFLICT (role_id, resource, action) DO NOTHING",
		},
		{
			name: "INSERT with ON CONFLICT DO UPDATE SET",
			sql:  "INSERT INTO roles (name, created_at) VALUES (${name}, now()) ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name",
			want: "INSERT INTO roles (\n    name,\n    created_at\n) VALUES (\n    ${name},\n    now()\n)\nON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name",
		},
		{
			name: "INSERT with ON CONFLICT DO UPDATE + RETURNING",
			sql:  "INSERT INTO roles (name) VALUES (${name}) ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING *",
			want: "INSERT INTO roles (\n    name\n) VALUES (\n    ${name}\n)\nON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name\nRETURNING *",
		},
		{
			name: "INSERT with ON CONFLICT DO NOTHING + RETURNING id",
			sql:  "INSERT INTO roles (name) VALUES (${name}) ON CONFLICT (name) DO NOTHING RETURNING id",
			want: "INSERT INTO roles (\n    name\n) VALUES (\n    ${name}\n)\nON CONFLICT (name) DO NOTHING\nRETURNING id",
		},
		{
			name: "INSERT literal containing ON CONFLICT text",
			sql:  "INSERT INTO logs (msg) VALUES ('a ON CONFLICT b')",
			want: "INSERT INTO logs (\n    msg\n) VALUES (\n    'a ON CONFLICT b'\n)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSQL(tt.sql)
			if got != tt.want {
				t.Errorf("FormatSQL():\n  got:  %q\n  want: %q", got, tt.want)
			}
		})
	}
}

func TestFormatSQL_Update(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{
			name: "UPDATE with multiple SET assignments",
			sql:  "UPDATE users SET name = ${name}, age = ${age}, updated_at = now() WHERE id = ${id}",
			want: "UPDATE users\nSET\n    name = ${name},\n    age = ${age},\n    updated_at = now()\nWHERE id = ${id}",
		},
		{
			name: "UPDATE with function args containing commas",
			sql:  "UPDATE users SET profile = jsonb_build_object('full_name', ${name}, 'note', 'a, b'), updated_at = now() WHERE id = ${id}",
			want: "UPDATE users\nSET\n    profile = jsonb_build_object('full_name', ${name}, 'note', 'a, b'),\n    updated_at = now()\nWHERE id = ${id}",
		},
		{
			name: "UPDATE with RETURNING and multiple WHERE conditions",
			sql:  "UPDATE users SET name = ${name}, age = ${age} WHERE id = ${id} AND tenant_id = ${tenantID} RETURNING *",
			want: "UPDATE users\nSET\n    name = ${name},\n    age = ${age}\nWHERE id = ${id}\n  AND tenant_id = ${tenantID}\nRETURNING *",
		},
		{
			name: "UPDATE with single SET assignment",
			sql:  "UPDATE users SET updated_at = now() WHERE id = ${id}",
			want: "UPDATE users\nSET updated_at = now()\nWHERE id = ${id}",
		},
		{
			name: "WITH + UPDATE FROM",
			sql:  "WITH due AS (SELECT id FROM settlement_retries WHERE done_at IS NULL AND dead_at IS NULL ORDER BY next_retry_at, id LIMIT ${limit} FOR UPDATE SKIP LOCKED) UPDATE settlement_retries SET attempts = attempts + 1, updated_at = NOW() FROM due WHERE settlement_retries.id = due.id RETURNING id",
			want: "WITH due AS (\n    SELECT id\n    FROM settlement_retries\n    WHERE done_at IS NULL\n      AND dead_at IS NULL\n    ORDER BY next_retry_at, id\n    LIMIT ${limit} FOR UPDATE SKIP LOCKED\n)\nUPDATE settlement_retries\nSET\n    attempts = attempts + 1,\n    updated_at = NOW()\nFROM due\nWHERE settlement_retries.id = due.id\nRETURNING id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSQL(tt.sql)
			if got != tt.want {
				t.Errorf("FormatSQL():\n  got:  %q\n  want: %q", got, tt.want)
			}
		})
	}
}
