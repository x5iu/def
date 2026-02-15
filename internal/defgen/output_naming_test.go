package defgen

import (
	"strings"
	"testing"
)

func TestImplOutputPathFromIntermediate(t *testing.T) {
	tests := []struct {
		name         string
		intermediate string
		want         string
	}{
		{name: "default", intermediate: "def_gen.go", want: "def_gen_impl.go"},
		{name: "relative path", intermediate: "gen/store.go", want: "gen/store_impl.go"},
		{name: "absolute path", intermediate: "/tmp/store.go", want: "/tmp/store_impl.go"},
		{name: "no ext", intermediate: "store", want: "store_impl.go"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := implOutputPathFromIntermediate(tt.intermediate); got != tt.want {
				t.Fatalf("implOutputPathFromIntermediate(%q) = %q, want %q", tt.intermediate, got, tt.want)
			}
		})
	}
}

func TestGenerateCode_UsesIntermediateBasedDefcOutputName(t *testing.T) {
	pkg := &Package{
		PkgName: "demo",
		Interfaces: map[string]*InterfaceInfo{
			"Store": {Name: "Store"},
		},
	}

	gotDefault, err := generateCode(pkg, &GenerateOptions{InterfaceName: "Store"})
	if err != nil {
		t.Fatalf("generateCode(default) error = %v", err)
	}
	if !strings.Contains(string(gotDefault), "-o def_gen_impl.go") {
		t.Fatalf("default go:generate output should be def_gen_impl.go.\nGot:\n%s", string(gotDefault))
	}

	gotCustom, err := generateCode(pkg, &GenerateOptions{InterfaceName: "Store", Output: "store.go"})
	if err != nil {
		t.Fatalf("generateCode(custom) error = %v", err)
	}
	if !strings.Contains(string(gotCustom), "-o store_impl.go") {
		t.Fatalf("custom go:generate output should be store_impl.go.\nGot:\n%s", string(gotCustom))
	}
}
