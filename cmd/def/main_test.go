package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
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
