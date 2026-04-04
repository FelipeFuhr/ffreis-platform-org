package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackendArgsIncludesLocalOverride(t *testing.T) {
	root := t.TempDir()
	stack := initRepoLayout(t, root, "prod")
	writeFile(t, filepath.Join(stack, "backend.local.hcl"), "bucket = \"local\"\n")

	args := backendArgs(stack, root, "prod")
	want := []string{
		"-backend-config=backend.local.hcl",
		"-backend-config=../envs/prod/backend.hcl",
	}
	if strings.Join(args, "|") != strings.Join(want, "|") {
		t.Fatalf("backend args: want %v got %v", want, args)
	}
}

func TestVarFileArgBuildsRelativePath(t *testing.T) {
	root := t.TempDir()
	stack := initRepoLayout(t, root, "prod")

	if got := varFileArg(stack, root, "prod"); got != "-var-file=../envs/prod/terraform.tfvars" {
		t.Fatalf("var file arg: got %q", got)
	}
}

func TestIsInitialised(t *testing.T) {
	stack := t.TempDir()
	if isInitialised(stack) {
		t.Fatal("expected stack without .terraform to be uninitialised")
	}
	if err := os.MkdirAll(filepath.Join(stack, ".terraform"), 0o755); err != nil {
		t.Fatalf("mkdir .terraform: %v", err)
	}
	if !isInitialised(stack) {
		t.Fatal("expected stack with .terraform to be initialised")
	}
}

func TestCaptureOutputReturnsStreamsAndExitCode(t *testing.T) {
	setupFakeTerraform(t, "printf 'stdout:%s\n' "$*"; printf 'stderr:%s\n' "$AWS_ACCESS_KEY_ID" >&2; exit 2")

	stdout, stderr, code, err := captureOutput(context.Background(), runOptions{
		stackPath: t.TempDir(),
		args:      []string{"plan", "-no-color"},
		creds: rawCreds{
			AccessKeyID:     "AKIAOUT",
			SecretAccessKey: "secret",
			SessionToken:    "token",
			Region:          "us-east-1",
		},
	})
	if err != nil {
		t.Fatalf("captureOutput: %v", err)
	}
	if code != 2 {
		t.Fatalf("exit code: want 2 got %d", code)
	}
	if !strings.Contains(stdout, "stdout:plan -no-color") {
		t.Fatalf("stdout missing args: %q", stdout)
	}
	if !strings.Contains(stderr, "stderr:AKIAOUT") {
		t.Fatalf("stderr missing env injection: %q", stderr)
	}
}

func TestTerraformInitUsesBackendArgs(t *testing.T) {
	root := t.TempDir()
	stack := initRepoLayout(t, root, "prod")
	writeFile(t, filepath.Join(stack, "backend.local.hcl"), "bucket = \"local\"\n")
	traceFile := filepath.Join(t.TempDir(), "trace.txt")
	t.Setenv("TRACE_FILE", traceFile)
	setupFakeTerraform(t, "printf '%s\n' "$*" > "$TRACE_FILE"")

	err := terraformInit(context.Background(), stack, root, "prod", rawCreds{Region: "us-east-1"})
	if err != nil {
		t.Fatalf("terraformInit: %v", err)
	}
	got, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("read trace file: %v", err)
	}
	want := "init -backend-config=backend.local.hcl -backend-config=../envs/prod/backend.hcl\n"
	if string(got) != want {
		t.Fatalf("terraform init args: want %q got %q", want, string(got))
	}
}

func TestEnsureInitSkipsWhenAlreadyInitialised(t *testing.T) {
	stack := t.TempDir()
	if err := os.MkdirAll(filepath.Join(stack, ".terraform"), 0o755); err != nil {
		t.Fatalf("mkdir .terraform: %v", err)
	}
	if err := ensureInit(context.Background(), stack, t.TempDir(), "prod", rawCreds{Region: "us-east-1"}); err != nil {
		t.Fatalf("ensureInit should skip: %v", err)
	}
}

func TestEnsureInitRunsInitWhenNeeded(t *testing.T) {
	d.log = newLogger("error")
	root := t.TempDir()
	stack := initRepoLayout(t, root, "prod")
	traceFile := filepath.Join(t.TempDir(), "trace.txt")
	t.Setenv("TRACE_FILE", traceFile)
	setupFakeTerraform(t, "printf '%s\n' "$1" > "$TRACE_FILE"")

	if err := ensureInit(context.Background(), stack, root, "prod", rawCreds{Region: "us-east-1"}); err != nil {
		t.Fatalf("ensureInit: %v", err)
	}
	got, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("read trace file: %v", err)
	}
	if string(got) != "init\n" {
		t.Fatalf("expected terraform init to run, got %q", string(got))
	}
}
