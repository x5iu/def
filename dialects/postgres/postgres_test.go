package postgres

import "testing"

func TestReturningPlaceholder(t *testing.T) {
	if Returning() != nil {
		t.Fatalf("Returning() should return nil placeholder")
	}
	if Returning("id") != nil {
		t.Fatalf("Returning(columns...) should return nil placeholder")
	}
}

func TestOnConflictPlaceholder(t *testing.T) {
	ct := OnConflict("id")
	if ct.DoNothing() != nil {
		t.Fatalf("DoNothing() should return nil placeholder")
	}
	if ct.DoUpdate("x") != nil {
		t.Fatalf("DoUpdate() should return nil placeholder")
	}
}

func TestExcludedPlaceholder(t *testing.T) {
	if Excluded("x") != nil {
		t.Fatalf("Excluded() should return nil placeholder")
	}
}

func TestAdditionalPlaceholders(t *testing.T) {
	if ForUpdateSkipLocked() != nil {
		t.Fatalf("ForUpdateSkipLocked() should return nil placeholder")
	}
	if Now() != "" {
		t.Fatalf("Now() should return empty placeholder string")
	}
	if Interval("10 minutes") != "" {
		t.Fatalf("Interval() should return empty placeholder string")
	}
}
