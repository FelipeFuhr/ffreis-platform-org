package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoRootAndStackDirUseWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	initRepoLayout(t, root, testEnv)
	writeFile(t, filepath.Join(root, ".git"), "gitdir: /fake\n")
	withWorkingDir(t, root)

	gotRoot, err := repoRoot()
	if err != nil {
		t.Fatalf("repoRoot: %v", err)
	}
	if gotRoot != root {
		t.Fatalf("repoRoot: want %q got %q", root, gotRoot)
	}

	gotStack, err := stackDir()
	if err != nil {
		t.Fatalf("stackDir: %v", err)
	}
	if gotStack != filepath.Join(root, stackDirName) {
		t.Fatalf("stackDir: want %q got %q", filepath.Join(root, stackDirName), gotStack)
	}
}

func TestRepoRootFindsRepositoryFromSubdirectory(t *testing.T) {
	root := t.TempDir()
	initRepoLayout(t, root, testEnv)
	writeFile(t, filepath.Join(root, ".git"), "gitdir: /fake\n")
	nested := filepath.Join(root, "terraform", "envs", testEnv)
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	withWorkingDir(t, nested)

	gotRoot, err := repoRoot()
	if err != nil {
		t.Fatalf("repoRoot from subdir: %v", err)
	}
	if gotRoot != root {
		t.Fatalf("repoRoot from subdir: want %q got %q", root, gotRoot)
	}
}

func TestRunTerraformReturnsExecErrorWhenBinaryMissing(t *testing.T) {
	t.Setenv("PATH", "")
	code, err := runTerraform(context.Background(), runOptions{
		stackPath: t.TempDir(),
		args:      []string{"plan"},
		creds:     rawCreds{Region: testRegion},
	})
	if err == nil || !strings.Contains(err.Error(), "running terraform") {
		t.Fatalf(errUnexpectedError, err)
	}
	if code != -1 {
		t.Fatalf("exit code: want -1 got %d", code)
	}
}

func TestTerraformInitReturnsExitCodeError(t *testing.T) {
	root := t.TempDir()
	stack := initRepoLayout(t, root, testEnv)
	setupFakeTerraform(t, `exit 1`)

	err := terraformInit(context.Background(), stack, root, testEnv, rawCreds{Region: testRegion})
	if err == nil || err.Error() != "terraform init exited with code 1" {
		t.Fatalf(errUnexpectedError, err)
	}
}
