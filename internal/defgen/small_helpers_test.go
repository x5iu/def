package defgen

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"
)

func TestTypeStringForLookup(t *testing.T) {
	if got := typeStringForLookup(nil); got != "<nil>" {
		t.Fatalf("typeStringForLookup(nil) = %q, want %q", got, "<nil>")
	}

	pkg := types.NewPackage("example.com/demo", "demo")
	obj := types.NewTypeName(token.NoPos, pkg, "User", nil)
	named := types.NewNamed(obj, types.NewStruct(nil, nil), nil)

	if got := typeStringForLookup(named); got != "example.com/demo.User" {
		t.Fatalf("typeStringForLookup(named) = %q, want %q", got, "example.com/demo.User")
	}
}

func TestSetCallName(t *testing.T) {
	tests := []struct {
		name    string
		fun     ast.Expr
		want    string
		wantErr bool
	}{
		{
			name: "ident",
			fun:  ast.NewIdent("now"),
			want: "now",
		},
		{
			name: "selector chain",
			fun: &ast.SelectorExpr{
				X:   ast.NewIdent("time"),
				Sel: ast.NewIdent("Now"),
			},
			want: "time.Now",
		},
		{
			name: "generic index expression",
			fun: &ast.IndexExpr{
				X: &ast.SelectorExpr{
					X:   ast.NewIdent("def"),
					Sel: ast.NewIdent("Func"),
				},
				Index: ast.NewIdent("string"),
			},
			want: "def.Func",
		},
		{
			name:    "unsupported",
			fun:     &ast.CallExpr{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := setCallName(tt.fun)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("setCallName() expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("setCallName() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("setCallName() = %q, want %q", got, tt.want)
			}
		})
	}
}
