package defgen

import (
	"strings"
	"testing"
)

func TestParseTxIsolationFlag(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		want  string
		isErr bool
	}{
		{name: "empty", raw: "", want: ""},
		{name: "default", raw: "default", want: "sql.LevelDefault"},
		{name: "read uncommitted", raw: "read-uncommitted", want: "sql.LevelReadUncommitted"},
		{name: "read committed", raw: "read-committed", want: "sql.LevelReadCommitted"},
		{name: "write committed", raw: "write-committed", want: "sql.LevelWriteCommitted"},
		{name: "repeatable read", raw: "repeatable-read", want: "sql.LevelRepeatableRead"},
		{name: "snapshot", raw: "snapshot", want: "sql.LevelSnapshot"},
		{name: "serializable", raw: "serializable", want: "sql.LevelSerializable"},
		{name: "linearizable", raw: "linearizable", want: "sql.LevelLinearizable"},
		{name: "invalid", raw: "foobar", isErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTxIsolationFlag(tt.raw)
			if tt.isErr {
				if err == nil {
					t.Fatalf("ParseTxIsolationFlag(%q) expected error", tt.raw)
				}
				if !strings.Contains(err.Error(), "allowed values") {
					t.Fatalf("ParseTxIsolationFlag(%q) error = %q, want allowed values hint", tt.raw, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseTxIsolationFlag(%q) error = %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("ParseTxIsolationFlag(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestSupportedTxIsolationValuesReturnsCopy(t *testing.T) {
	values := SupportedTxIsolationValues()
	if len(values) == 0 {
		t.Fatal("SupportedTxIsolationValues() returned empty slice")
	}
	values[0] = "mutated"
	fresh := SupportedTxIsolationValues()
	if fresh[0] == "mutated" {
		t.Fatal("SupportedTxIsolationValues() should return a copy")
	}
}
