package defgen

import (
	"bytes"
	"fmt"
	"go/token"
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

	// Should use *User (actual type) not *Author (field name) in getCache/setCache
	if strings.Contains(got, "*Author") {
		t.Errorf("generateCallbackMethod() should use type name 'User', not field name 'Author'.\nGot:\n%s", got)
	}
	if !strings.Contains(got, "getCache[*User]") {
		t.Errorf("generateCallbackMethod() should contain getCache[*User].\nGot:\n%s", got)
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

	// Should use alias type name "ModelEndpointRows" for getCache/setCache, not field name "Endpoints"
	if !strings.Contains(got, "getCache[ModelEndpointRows]") {
		t.Errorf("should use alias type name in getCache, not field name.\nGot:\n%s", got)
	}
	if strings.Contains(got, "getCache[Endpoints]") {
		t.Errorf("should NOT use field name 'Endpoints' as type in getCache.\nGot:\n%s", got)
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
