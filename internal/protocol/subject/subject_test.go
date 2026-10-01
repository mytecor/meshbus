package subject

import (
	"errors"
	"testing"
)

func TestValidationAndMatching(t *testing.T) {
	if err := ValidateTopic("jobs.completed"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTopic("jobs.*"); !errors.Is(err, ErrInvalidTopic) {
		t.Fatalf("topic wildcard error = %v", err)
	}
	if err := ValidatePattern("jobs.>.invalid"); !errors.Is(err, ErrInvalidPattern) {
		t.Fatalf("terminal wildcard error = %v", err)
	}
	if !Match("jobs.*", "jobs.completed") || !Match(">", "jobs.completed") || Match("jobs.*", "jobs") {
		t.Fatal("unexpected subject match result")
	}
}
