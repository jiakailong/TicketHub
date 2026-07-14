package cache

import "testing"

func TestKeyBuilder(t *testing.T) {
	builder := NewKeyBuilder("tickethub")
	got := builder.Build("program", 1, "ticket", 2, "remain")
	want := "tickethub:program:1:ticket:2:remain"
	if got != want {
		t.Fatalf("Build() = %s, want %s", got, want)
	}
}
