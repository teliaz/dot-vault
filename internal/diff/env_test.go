package diff

import "testing"

func TestParseEnv(t *testing.T) {
	t.Parallel()

	env, err := ParseEnv([]byte("# comment\nexport API_KEY=abc\nEMPTY=\nSPACED = value \n"))
	if err != nil {
		t.Fatalf("ParseEnv() error = %v", err)
	}

	if env.Values["API_KEY"] != "abc" {
		t.Fatalf("API_KEY = %q, want abc", env.Values["API_KEY"])
	}
	if env.Values["EMPTY"] != "" {
		t.Fatalf("EMPTY = %q, want empty string", env.Values["EMPTY"])
	}
	if env.Values["SPACED"] != "value" {
		t.Fatalf("SPACED = %q, want value", env.Values["SPACED"])
	}
}

func TestCompare(t *testing.T) {
	t.Parallel()

	stored := Env{Values: map[string]string{
		"OLD":     "stored",
		"SAME":    "same",
		"CHANGED": "stored",
	}}
	current := Env{Values: map[string]string{
		"NEW":     "current",
		"SAME":    "same",
		"CHANGED": "current",
	}}

	result := Compare(stored, current)
	if result.AddedCount() != 1 {
		t.Fatalf("AddedCount() = %d, want 1", result.AddedCount())
	}
	if result.RemovedCount() != 1 {
		t.Fatalf("RemovedCount() = %d, want 1", result.RemovedCount())
	}
	if result.ChangedCount() != 1 {
		t.Fatalf("ChangedCount() = %d, want 1", result.ChangedCount())
	}
	if result.UnchangedCount() != 1 {
		t.Fatalf("UnchangedCount() = %d, want 1", result.UnchangedCount())
	}
	if !result.HasDrift() {
		t.Fatalf("HasDrift() = false, want true")
	}
}
