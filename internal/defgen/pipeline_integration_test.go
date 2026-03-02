package defgen

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGenerate_EndToEndPipeline(t *testing.T) {
	t.Setenv("GOFLAGS", "-mod=mod")

	moduleDir, pkgDir := writePipelineModule(t)

	err := Generate(moduleDir, "store", &GenerateOptions{
		Output:        "store_gen.go",
		InterfaceName: "Store",
		Tags:          "def,unit",
		DefcFeatures:  "sqlx/rebind",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	generatedPath := filepath.Join(pkgDir, "store_gen.go")
	gotBytes, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatalf("failed to read generated file %s: %v", generatedPath, err)
	}
	got := string(gotBytes)

	wants := []string{
		"//go:build !def && !unit",
		"--features sqlx/callback,sqlx/rebind",
		"// FindUser query constbind",
		"// CreateUserIgnore exec constbind",
		"ON CONFLICT (project_id, title) DO UPDATE SET score = EXCLUDED.score",
		"WITH due AS (",
		"getUserByID(ctx context.Context,",
		"func (u *User) Callback(ctx context.Context, q Store) error",
		"type Projects []*Project",
		"func (p *Projects) Callback(ctx context.Context, q Store) error",
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("generated code missing %q\n--- generated ---\n%s", want, got)
		}
	}
}

func TestGenerate_DefaultOutputAndAutoInterface(t *testing.T) {
	t.Setenv("GOFLAGS", "-mod=mod")

	moduleDir, pkgDir := writePipelineModule(t)

	if err := Generate(moduleDir, "store", &GenerateOptions{}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	generatedPath := filepath.Join(pkgDir, "def_gen.go")
	gotBytes, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatalf("failed to read generated file %s: %v", generatedPath, err)
	}
	got := string(gotBytes)

	if !strings.Contains(got, "type Store interface {") {
		t.Fatalf("generated code should include Store interface\n--- generated ---\n%s", got)
	}
	if !strings.Contains(got, "-o def_gen_impl.go") {
		t.Fatalf("generated code should use default implementation output name\n--- generated ---\n%s", got)
	}
}

func TestGenerate_RejectsAbsoluteOutputForMultiplePackages(t *testing.T) {
	t.Setenv("GOFLAGS", "-mod=mod")

	moduleDir := t.TempDir()
	goMod := "module example.com/multi\n\ngo 1.22\n"
	if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	for _, pkg := range []string{"a", "b"} {
		dir := filepath.Join(moduleDir, pkg)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create package dir %s: %v", dir, err)
		}
		src := fmt.Sprintf("package %s\n\ntype X struct{}\n", pkg)
		if err := os.WriteFile(filepath.Join(dir, pkg+".go"), []byte(src), 0o644); err != nil {
			t.Fatalf("failed to write %s source: %v", pkg, err)
		}
	}

	err := Generate(moduleDir, "./...", &GenerateOptions{
		Output: filepath.Join(moduleDir, "out.go"),
	})
	if err == nil {
		t.Fatalf("Generate() expected error for absolute output with multiple packages")
	}
	if !strings.Contains(err.Error(), "absolute output path is not supported") {
		t.Fatalf("Generate() error = %v, want absolute output path error", err)
	}
}

func TestDetermineOutputPath(t *testing.T) {
	pkg := &Package{Dir: "/tmp/demo"}

	gotDefault, err := determineOutputPath(pkg, "")
	if err != nil {
		t.Fatalf("determineOutputPath(default) error = %v", err)
	}
	if gotDefault != "/tmp/demo/def_gen.go" {
		t.Fatalf("determineOutputPath(default) = %q, want %q", gotDefault, "/tmp/demo/def_gen.go")
	}

	gotRelative, err := determineOutputPath(pkg, "custom.go")
	if err != nil {
		t.Fatalf("determineOutputPath(relative) error = %v", err)
	}
	if gotRelative != "/tmp/demo/custom.go" {
		t.Fatalf("determineOutputPath(relative) = %q, want %q", gotRelative, "/tmp/demo/custom.go")
	}

	gotAbsolute, err := determineOutputPath(pkg, "/tmp/custom.go")
	if err != nil {
		t.Fatalf("determineOutputPath(absolute) error = %v", err)
	}
	if gotAbsolute != "/tmp/custom.go" {
		t.Fatalf("determineOutputPath(absolute) = %q, want %q", gotAbsolute, "/tmp/custom.go")
	}
}

func writePipelineModule(t *testing.T) (moduleDir, pkgDir string) {
	t.Helper()

	moduleDir = t.TempDir()
	pkgDir = filepath.Join(moduleDir, "store")

	repoRoot := repositoryRoot(t)
	goMod := fmt.Sprintf(`module example.com/pipeline

go 1.22

require github.com/x5iu/def v0.0.0

replace github.com/x5iu/def => %s
`, repoRoot)
	if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("failed to create package dir: %v", err)
	}

	source := strings.ReplaceAll(`package store

import (
	"context"
	"database/sql"

	"github.com/x5iu/def"
	"github.com/x5iu/def/dialects/postgres"
)

type Projects []*Project
type Tasks []*Task

type User struct {
	ID       int64    {{BT}}db:"id" primary_key:"true"{{BT}}
	Name     string   {{BT}}db:"name"{{BT}}
	Projects Projects {{BT}}db:"-"{{BT}}
}

type Project struct {
	ID     int64   {{BT}}db:"id" primary_key:"true"{{BT}}
	UserID int64   {{BT}}db:"user_id"{{BT}}
	Name   string  {{BT}}db:"name"{{BT}}
	User   *User   {{BT}}db:"-" foreign_key:"user_id" inverse:"Projects"{{BT}}
	Tasks  []*Task {{BT}}db:"-"{{BT}}
}

type Task struct {
	ID        int64    {{BT}}db:"id" primary_key:"true"{{BT}}
	ProjectID int64    {{BT}}db:"project_id"{{BT}}
	Title     string   {{BT}}db:"title"{{BT}}
	Score     int      {{BT}}db:"score"{{BT}}
	Project   *Project {{BT}}db:"-" foreign_key:"project_id" inverse:"Tasks"{{BT}}
}

var (
	user    = &User{}
	project = &Project{}
	task    = &Task{}
	due     = &Task{}
)

var _ = def.Init(
	def.BindTable[User]("users"),
	def.BindTable[Project]("projects"),
	def.BindTable[Task]("tasks"),
)

type Store interface {
	WithTx(ctx context.Context, fn func(Store) error) error
	FindUser(ctx context.Context, id int64, names []string, limit int, offset int) (*User, error)
	FindProjectsByUserName(ctx context.Context, userName string) ([]*Project, error)
	SaveUser(ctx context.Context, u *User) (sql.Result, error)
	CreateProject(ctx context.Context, p *Project) (*Project, error)
	CreateUserIgnore(ctx context.Context, id int64, name string) (sql.Result, error)
	UpsertTask(ctx context.Context, projectID int64, title string, score int) (*Task, error)
	ClaimTasks(ctx context.Context, limit int) ([]*Task, error)
	RemoveTask(ctx context.Context, id int64) (sql.Result, error)
}

type repo struct{}

func (r *repo) WithTx(ctx context.Context, fn func(Store) error) error { return nil }

func (r *repo) FindUser(ctx context.Context, id int64, names []string, limit int, offset int) (*User, error) {
	def.Query(
		def.Column(user.ID),
		def.Column(def.Func[string]("COALESCE", user.Name, "unknown")),
		def.Filter(user.ID == id && def.In(user.Name, names)),
		def.Limit(limit),
		def.Offset(offset),
	)
	return nil, nil
}

func (r *repo) FindProjectsByUserName(ctx context.Context, userName string) ([]*Project, error) {
	def.Query(
		def.Column(project.ID),
		def.Column(def.Count[int64](project.ID)),
		def.Filter(project.User.Name == userName),
	)
	return nil, nil
}

func (r *repo) SaveUser(ctx context.Context, u *User) (sql.Result, error) {
	def.Update(
		u,
		def.Filter(u.ID == u.ID),
	)
	return nil, nil
}

func (r *repo) CreateProject(ctx context.Context, p *Project) (*Project, error) {
	def.Create(p, postgres.Returning())
	return nil, nil
}

func (r *repo) CreateUserIgnore(ctx context.Context, id int64, name string) (sql.Result, error) {
	def.Create(
		def.Set(user.ID, id),
		def.Set(user.Name, name),
		postgres.OnConflict(user.ID).DoNothing(),
	)
	return nil, nil
}

func (r *repo) UpsertTask(ctx context.Context, projectID int64, title string, score int) (*Task, error) {
	def.Create(
		def.Set(task.ProjectID, projectID),
		def.Set(task.Title, title),
		def.Set(task.Score, score),
		postgres.OnConflict(task.ProjectID, task.Title).DoUpdate(
			def.Set(task.Score, postgres.Excluded(task.Score)),
		),
		postgres.Returning(task.ID, task.ProjectID),
	)
	return nil, nil
}

func (r *repo) ClaimTasks(ctx context.Context, limit int) ([]*Task, error) {
	def.Update(
		def.Set(task.Score, task.Score+1),
		def.Set(task.Title, def.Func[string]("LOWER", task.Title)),
		def.With(
			"due",
			def.From(task),
			def.Column(task.ID),
			def.Filter(def.IsNull(task.Title) || def.IsNotNull(task.ProjectID)),
			def.OrderBy(def.Asc(task.ID)),
			def.Limit(limit),
			postgres.ForUpdateSkipLocked(),
		),
		def.From("due"),
		def.Filter(task.ID == due.ID),
		postgres.Returning(),
	)
	return nil, nil
}

func (r *repo) RemoveTask(ctx context.Context, id int64) (sql.Result, error) {
	def.Delete(def.Filter(task.ID == id))
	return nil, nil
}
`, "{{BT}}", "`")

	if err := os.WriteFile(filepath.Join(pkgDir, "store.go"), []byte(source), 0o644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	return moduleDir, pkgDir
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller() failed")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
