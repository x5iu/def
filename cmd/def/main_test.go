package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/x5iu/def/internal/defgen"
)

func TestNewRootCommand_Help(t *testing.T) {
	cmd := newRootCommand()
	cmd.SetArgs([]string{"--help"})

	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(--help) error = %v", err)
	}
	if !strings.Contains(output.String(), "generate") {
		t.Fatalf("help output should mention generate subcommand")
	}
}

func TestGenerateCommandHelpIncludesTxIsolation(t *testing.T) {
	cmd := newRootCommand()
	cmd.SetArgs([]string{"generate", "--help"})

	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(generate --help) error = %v", err)
	}
	if !strings.Contains(output.String(), "tx-isolation") {
		t.Fatalf("generate help output should mention tx-isolation flag")
	}
	if !strings.Contains(output.String(), "tx") {
		t.Fatalf("generate help output should mention tx flag")
	}
	if !strings.Contains(output.String(), "tx-type") {
		t.Fatalf("generate help output should mention tx-type flag")
	}
}

func TestBadFlagPrintsError(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "--badflag")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit for invalid flag")
	}

	output := stderr.String()
	if !strings.Contains(output, "unknown flag") {
		t.Fatalf("stderr = %q, want unknown flag message", output)
	}
}

func TestMain_HelpDoesNotExit(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"def", "--help"}
	main()
}

func TestGenerateCommandExecutePath(t *testing.T) {
	cmd := newRootCommand()
	cmd.SetArgs([]string{
		"generate",
		"--tx-isolation", "serializable",
		"--tx",
		"--tx-type", "TxStore",
		".",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(generate) error = %v", err)
	}
}

func TestExecuteGenerate_DefaultPattern(t *testing.T) {
	oldGetwd := osGetwd
	oldParse := parseTxFlag
	oldRun := runGenerate
	defer func() {
		osGetwd = oldGetwd
		parseTxFlag = oldParse
		runGenerate = oldRun
	}()

	var gotPattern string
	osGetwd = func() (string, error) { return "/tmp/work", nil }
	parseTxFlag = func(string) (string, error) { return "sql.LevelDefault", nil }
	runGenerate = func(_, pattern string, _ *defgen.GenerateOptions) error {
		gotPattern = pattern
		return nil
	}

	opts := &defgen.GenerateOptions{}
	if err := executeGenerate(nil, "", opts); err != nil {
		t.Fatalf("executeGenerate() error = %v", err)
	}
	if gotPattern != "." {
		t.Fatalf("executeGenerate() pattern = %q, want %q", gotPattern, ".")
	}
	if opts.TxIsolation != "sql.LevelDefault" {
		t.Fatalf("executeGenerate() tx isolation = %q, want %q", opts.TxIsolation, "sql.LevelDefault")
	}
}

func TestExecuteGenerate_GetwdError(t *testing.T) {
	oldGetwd := osGetwd
	defer func() { osGetwd = oldGetwd }()

	osGetwd = func() (string, error) { return "", errors.New("wd err") }
	err := executeGenerate([]string{"."}, "", &defgen.GenerateOptions{})
	if err == nil {
		t.Fatalf("executeGenerate() expected error")
	}
	if !strings.Contains(err.Error(), "failed to get working directory") {
		t.Fatalf("executeGenerate() error = %v, want getwd error", err)
	}
}

func TestExecuteGenerate_ParseIsolationError(t *testing.T) {
	oldGetwd := osGetwd
	oldParse := parseTxFlag
	defer func() {
		osGetwd = oldGetwd
		parseTxFlag = oldParse
	}()

	osGetwd = func() (string, error) { return "/tmp/work", nil }
	parseTxFlag = func(string) (string, error) { return "", errors.New("bad isolation") }

	err := executeGenerate([]string{"."}, "bad", &defgen.GenerateOptions{})
	if err == nil {
		t.Fatalf("executeGenerate() expected error")
	}
	if !strings.Contains(err.Error(), "generate failed") {
		t.Fatalf("executeGenerate() error = %v, want generate failed", err)
	}
}

func TestExecuteGenerate_GenerateError(t *testing.T) {
	oldGetwd := osGetwd
	oldParse := parseTxFlag
	oldRun := runGenerate
	defer func() {
		osGetwd = oldGetwd
		parseTxFlag = oldParse
		runGenerate = oldRun
	}()

	osGetwd = func() (string, error) { return "/tmp/work", nil }
	parseTxFlag = func(string) (string, error) { return "", nil }
	runGenerate = func(_, _ string, _ *defgen.GenerateOptions) error {
		return errors.New("boom")
	}

	err := executeGenerate([]string{"."}, "", &defgen.GenerateOptions{})
	if err == nil {
		t.Fatalf("executeGenerate() expected error")
	}
	if !strings.Contains(err.Error(), "generate failed") {
		t.Fatalf("executeGenerate() error = %v, want generate failed", err)
	}
}

func TestMain_ExecuteErrorPath(t *testing.T) {
	oldArgs := os.Args
	oldExit := osExit
	defer func() {
		os.Args = oldArgs
		osExit = oldExit
	}()

	exitCode := -1
	osExit = func(code int) {
		exitCode = code
		panic("exit")
	}

	os.Args = []string{"def", "unknown-subcommand"}
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("main() expected osExit panic")
		}
		if exitCode != 1 {
			t.Fatalf("main() exit code = %d, want 1", exitCode)
		}
	}()

	main()
}
