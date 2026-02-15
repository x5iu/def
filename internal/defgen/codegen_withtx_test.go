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

func TestGenerateCode_WithTxForcedWithoutSourceMethod(t *testing.T) {
	pkg := &Package{
		PkgName: "testpkg",
		Interfaces: map[string]*InterfaceInfo{
			"Store": {Name: "Store"},
		},
	}

	gotBytes, err := generateCode(pkg, &GenerateOptions{
		InterfaceName: "Store",
		WithTx:        true,
	})
	if err != nil {
		t.Fatalf("generateCode() error = %v", err)
	}
	got := string(gotBytes)
	if !strings.Contains(got, "WithTx(ctx context.Context, fn func(Store) error) error") {
		t.Fatalf("forced WithTx should be generated with interface name.\nGot:\n%s", got)
	}
}

func TestGenerateCode_WithTxForcedWithCustomFnType(t *testing.T) {
	pkg := &Package{
		PkgName: "testpkg",
		Interfaces: map[string]*InterfaceInfo{
			"Store": {Name: "Store"},
		},
	}

	gotBytes, err := generateCode(pkg, &GenerateOptions{
		InterfaceName: "Store",
		WithTx:        true,
		WithTxFnType:  "TxStore",
	})
	if err != nil {
		t.Fatalf("generateCode() error = %v", err)
	}
	got := string(gotBytes)
	if !strings.Contains(got, "WithTx(ctx context.Context, fn func(TxStore) error) error") {
		t.Fatalf("forced WithTx should use custom fn type.\nGot:\n%s", got)
	}
}

func TestGenerateCode_WithTxFnTypeOverridesSourceMethod(t *testing.T) {
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
		WithTxFnType:  "TxStore",
	})
	if err != nil {
		t.Fatalf("generateCode() error = %v", err)
	}
	got := string(gotBytes)
	if !strings.Contains(got, "WithTx(ctx context.Context, fn func(TxStore) error) error") {
		t.Fatalf("WithTx should use overridden fn type.\nGot:\n%s", got)
	}
}

func TestGenerateCode_WithTxFnTypeRequiresWithTxOrForce(t *testing.T) {
	pkg := &Package{
		PkgName: "testpkg",
		Interfaces: map[string]*InterfaceInfo{
			"Store": {Name: "Store"},
		},
	}

	_, err := generateCode(pkg, &GenerateOptions{
		InterfaceName: "Store",
		WithTxFnType:  "TxStore",
	})
	if err == nil {
		t.Fatal("generateCode() expected error when --tx-type is set without WithTx or --tx")
	}
	if !strings.Contains(err.Error(), "--tx-type requires WithTx method or --tx") {
		t.Fatalf("generateCode() error = %v, want --tx-type requires WithTx method or --tx", err)
	}
}

func TestGenerateCode_TxIsolationWithForcedWithTx(t *testing.T) {
	pkg := &Package{
		PkgName: "testpkg",
		Interfaces: map[string]*InterfaceInfo{
			"Store": {Name: "Store"},
		},
	}

	gotBytes, err := generateCode(pkg, &GenerateOptions{
		InterfaceName: "Store",
		WithTx:        true,
		TxIsolation:   "sql.LevelSerializable",
	})
	if err != nil {
		t.Fatalf("generateCode() error = %v", err)
	}
	got := string(gotBytes)
	if !strings.Contains(got, "// WithTx ISOLATION=sql.LevelSerializable") {
		t.Fatalf("generated interface should include WithTx isolation comment when WithTx is forced.\nGot:\n%s", got)
	}
}
