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
