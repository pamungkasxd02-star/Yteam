package command

import "testing"

func TestCanonicalAliasesMatchOpenCodeSlashSurface(t *testing.T) {
	for input, want := range map[string]string{
		"/q": "exit", "/quit": "exit", "/resume": "sessions", "/continue": "sessions",
		"/clear": "new", "/variant": "variants", "/agent": "agents", "/debug": "status",
	} {
		if got := Canonical(input); got != want {
			t.Fatalf("Canonical(%q) = %q, want %q", input, got, want)
		}
	}
}
