package defgen

import (
	"fmt"
	"strings"
)

var txIsolationValues = []string{
	"default",
	"read-uncommitted",
	"read-committed",
	"write-committed",
	"repeatable-read",
	"snapshot",
	"serializable",
	"linearizable",
}

var txIsolationMap = map[string]string{
	"default":          "sql.LevelDefault",
	"read-uncommitted": "sql.LevelReadUncommitted",
	"read-committed":   "sql.LevelReadCommitted",
	"write-committed":  "sql.LevelWriteCommitted",
	"repeatable-read":  "sql.LevelRepeatableRead",
	"snapshot":         "sql.LevelSnapshot",
	"serializable":     "sql.LevelSerializable",
	"linearizable":     "sql.LevelLinearizable",
}

// SupportedTxIsolationValues returns the supported values for --tx-isolation.
func SupportedTxIsolationValues() []string {
	values := make([]string, len(txIsolationValues))
	copy(values, txIsolationValues)
	return values
}

// ParseTxIsolationFlag validates and maps a --tx-isolation CLI value.
// Empty input returns an empty string.
func ParseTxIsolationFlag(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}

	mapped, ok := txIsolationMap[raw]
	if ok {
		return mapped, nil
	}

	return "", fmt.Errorf("invalid --tx-isolation value %q; allowed values: %s", raw, strings.Join(txIsolationValues, ", "))
}
