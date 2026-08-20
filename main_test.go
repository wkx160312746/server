package main

import "testing"

func TestEnvInt(t *testing.T) {
	t.Setenv("PORT", "8080")
	if got := envInt("PORT", 9998); got != 8080 {
		t.Fatalf("envInt() = %d, want 8080", got)
	}

	t.Setenv("PORT", "invalid")
	if got := envInt("PORT", 9998); got != 9998 {
		t.Fatalf("envInt() fallback = %d, want 9998", got)
	}
}

func TestEnvInt64(t *testing.T) {
	t.Setenv("BOT_GROUP", "123456789")
	if got := envInt64("BOT_GROUP", 0); got != 123456789 {
		t.Fatalf("envInt64() = %d, want 123456789", got)
	}

	t.Setenv("BOT_GROUP", "invalid")
	if got := envInt64("BOT_GROUP", 42); got != 42 {
		t.Fatalf("envInt64() fallback = %d, want 42", got)
	}
}
