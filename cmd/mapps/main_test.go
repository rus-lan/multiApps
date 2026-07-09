package main

import "testing"

func TestRun_NoArgs(t *testing.T) {
	if got := run(nil); got != 2 {
		t.Errorf("run(nil) = %d, want 2", got)
	}
}

func TestRun_Help(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		if got := run([]string{arg}); got != 0 {
			t.Errorf("run([%q]) = %d, want 0", arg, got)
		}
	}
}

func TestRun_UnknownSubcommand(t *testing.T) {
	if got := run([]string{"bogus"}); got != 2 {
		t.Errorf("run([bogus]) = %d, want 2", got)
	}
}

func TestRun_AddArity(t *testing.T) {
	cases := [][]string{
		{"add"},
		{"add", "url", "dir", "branch", "extra"},
	}
	for _, args := range cases {
		if got := run(args); got != 2 {
			t.Errorf("run(%v) = %d, want 2", args, got)
		}
	}
}

func TestRun_RmUsageErrors(t *testing.T) {
	cases := [][]string{
		{"rm"},
		{"rm", "--force"},
		{"rm", "a", "b"},
		{"rm", "-x", "a"},
	}
	for _, args := range cases {
		if got := run(args); got != 2 {
			t.Errorf("run(%v) = %d, want 2", args, got)
		}
	}
}
