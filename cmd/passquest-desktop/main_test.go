package main

import "testing"

func TestRunDispatch(t *testing.T) {
	windows := 0
	window := func() int {
		windows++
		return 7
	}
	if code := run([]string{"version"}, window); code != 0 || windows != 0 {
		t.Fatalf("CLI arguments: exit %d, window opened %d times", code, windows)
	}
	if code := run([]string{"no-such-command"}, window); code == 0 || windows != 0 {
		t.Fatalf("unknown command: exit %d, window opened %d times", code, windows)
	}
	if code := run(nil, window); code != 7 || windows != 1 {
		t.Fatalf("no arguments: exit %d, window opened %d times", code, windows)
	}
}
