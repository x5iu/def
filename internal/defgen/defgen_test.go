package defgen

import (
	"go/token"
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
