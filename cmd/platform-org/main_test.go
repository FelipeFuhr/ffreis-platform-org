package main

import (
	"errors"
	"testing"
)

func TestMainDoesNotExitOnSuccess(t *testing.T) {
	oldExecute := execute
	oldExit := exitFunc
	t.Cleanup(func() {
		execute = oldExecute
		exitFunc = oldExit
	})

	exited := false
	execute = func() error { return nil }
	exitFunc = func(int) { exited = true }

	main()

	if exited {
		t.Fatal("main should not exit when execute succeeds")
	}
}

func TestMainExitsOnError(t *testing.T) {
	oldExecute := execute
	oldExit := exitFunc
	t.Cleanup(func() {
		execute = oldExecute
		exitFunc = oldExit
	})

	code := 0
	execute = func() error { return errors.New("boom") }
	exitFunc = func(c int) { code = c }

	main()

	if code != 1 {
		t.Fatalf("exit code: want 1 got %d", code)
	}
}
