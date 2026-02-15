package defgen

import (
	"strings"
	"testing"
)

func TestGenerateCode_WithTxFromSpecifiedInterface(t *testing.T) {
	pkg := &Package{
		PkgName: "testpkg",
		Interfaces: map[string]*InterfaceInfo{
			"Store": {
				Name: "Store",
				Methods: []InterfaceMethod{
					{
						Name:      "WithTx",
						Signature: "(ctx context.Context, fn func(Store) error) error",
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
	if !strings.Contains(got, "WithTx(ctx context.Context, fn func(Store) error) error") {
		t.Fatalf("generated interface should include WithTx from specified source interface.\nGot:\n%s", got)
	}
}

func TestGenerateCode_WithTxIsolationComment(t *testing.T) {
	pkg := &Package{
		PkgName: "testpkg",
		Interfaces: map[string]*InterfaceInfo{
			"Store": {
				Name: "Store",
				Methods: []InterfaceMethod{
					{
						Name:      "WithTx",
						Signature: "(ctx context.Context, fn func(Store) error) error",
					},
				},
			},
		},
	}

	gotBytes, err := generateCode(pkg, &GenerateOptions{
		InterfaceName: "Store",
		TxIsolation:   "sql.LevelSerializable",
	})
	if err != nil {
		t.Fatalf("generateCode() error = %v", err)
	}
	got := string(gotBytes)
	if !strings.Contains(got, "// WithTx ISOLATION=sql.LevelSerializable") {
		t.Fatalf("generated interface should include WithTx isolation comment.\nGot:\n%s", got)
	}
}

func TestGenerateCode_TxIsolationRequiresWithTx(t *testing.T) {
	pkg := &Package{
		PkgName: "testpkg",
		Interfaces: map[string]*InterfaceInfo{
			"Store": {
				Name:    "Store",
				Methods: nil,
			},
		},
	}

	_, err := generateCode(pkg, &GenerateOptions{
		InterfaceName: "Store",
		TxIsolation:   "sql.LevelSerializable",
	})
	if err == nil {
		t.Fatal("generateCode() expected error when --tx-isolation is set without WithTx")
	}
	if !strings.Contains(err.Error(), "--tx-isolation requires WithTx method") {
		t.Fatalf("generateCode() error = %v, want --tx-isolation requires WithTx method", err)
	}
}
