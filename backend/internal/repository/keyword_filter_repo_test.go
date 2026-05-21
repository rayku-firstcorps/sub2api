package repository

import "testing"

func TestEscapePostgresLike(t *testing.T) {
	got := escapePostgresLike(`a\b%c_d`)
	want := `a\\b\%c\_d`
	if got != want {
		t.Fatalf("escapePostgresLike() = %q, want %q", got, want)
	}
}
