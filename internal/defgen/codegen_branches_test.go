package defgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateNilOptionsAndNoMethodPackage(t *testing.T) {
	t.Setenv("GOFLAGS", "-mod=mod")

	moduleDir := t.TempDir()
	repoRoot := repositoryRoot(t)
	goMod := "module example.com/nomethod\n\ngo 1.22\n\nrequire github.com/x5iu/def v0.0.0\n\nreplace github.com/x5iu/def => " + repoRoot + "\n"
	if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	pkgDir := filepath.Join(moduleDir, "store")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("failed to create package dir: %v", err)
	}

	source := strings.ReplaceAll(`package store

import "github.com/x5iu/def"

type User struct {
	ID int64 {{BT}}db:"id" primary_key:"true"{{BT}}
}

var _ = def.Init(def.BindTable[User]("users"))
`, "{{BT}}", "`")
	if err := os.WriteFile(filepath.Join(pkgDir, "store.go"), []byte(source), 0o644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	if err := Generate(moduleDir, "store", nil); err != nil {
		t.Fatalf("Generate(nil opts) error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(pkgDir, "def_gen.go")); !os.IsNotExist(err) {
		t.Fatalf("Generate() should skip writing output when no methods, stat err=%v", err)
	}
}

func TestGenerateErrorBranches(t *testing.T) {
	t.Setenv("GOFLAGS", "-mod=mod")

	_, pkgDir := writePipelineModule(t)

	err := Generate(pkgDir, "missing-package-pattern", &GenerateOptions{})
	if err == nil {
		t.Fatalf("Generate() expected load error for missing package")
	}

	err = Generate(filepath.Dir(pkgDir), "store", &GenerateOptions{Output: filepath.Join("missing", "out.go"), InterfaceName: "Store"})
	if err == nil || !strings.Contains(err.Error(), "failed to write output file") {
		t.Fatalf("Generate() error = %v, want write output error", err)
	}
}

func TestGenerateParseFailure(t *testing.T) {
	t.Setenv("GOFLAGS", "-mod=mod")

	moduleDir := t.TempDir()
	repoRoot := repositoryRoot(t)
	goMod := "module example.com/badparse\n\ngo 1.22\n\nrequire github.com/x5iu/def v0.0.0\n\nreplace github.com/x5iu/def => " + repoRoot + "\n"
	if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	pkgDir := filepath.Join(moduleDir, "store")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("failed to create package dir: %v", err)
	}

	source := strings.ReplaceAll(`package store

import (
	"context"
	"github.com/x5iu/def"
)

type User struct {
	ID int64 {{BT}}db:"id" primary_key:"true"{{BT}}
}

var user = &User{}
var _ = def.Init(def.BindTable[User]("users"))

type Store interface {
	Bad(ctx context.Context, id int64) (*User, error)
}

type repo struct{}

func customFilter(ok bool) bool { return ok }

func (r *repo) Bad(ctx context.Context, id int64) (*User, error) {
	def.Query(def.Filter(customFilter(user.ID == id)))
	return nil, nil
}
`, "{{BT}}", "`")
	if err := os.WriteFile(filepath.Join(pkgDir, "store.go"), []byte(source), 0o644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	err := Generate(moduleDir, "store", &GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), "unsupported call expression in filter") {
		t.Fatalf("Generate() error = %v, want parse failure", err)
	}
}

func TestInvokeDefcWithoutInterfaceName(t *testing.T) {
	err := invokeDefc("store.go", []byte("package store\n"), &Package{
		Interfaces:      map[string]*InterfaceInfo{},
		Methods:         nil,
		MutationMethods: nil,
	}, &GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), "cannot determine interface name") {
		t.Fatalf("invokeDefc() error = %v, want cannot determine interface", err)
	}
}
