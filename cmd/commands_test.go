package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPlanCommandAllowsDetailedExitCodeTwo(t *testing.T) {
	d.log = newLogger("error")
	d.env = "prod"
	d.creds = rawCreds{Region: "us-east-1"}
	root := t.TempDir()
	stack := initRepoLayout(t, root, "prod")
	if err := os.MkdirAll(filepath.Join(stack, ".terraform"), 0o755); err != nil {
		t.Fatalf("mkdir .terraform: %v", err)
	}
	traceFile := filepath.Join(t.TempDir(), "trace.txt")
	t.Setenv("TRACE_FILE", traceFile)
	setupFakeTerraform(t, "printf '%s\n' "$*" > "$TRACE_FILE"; exit 2")
	withWorkingDir(t, root)
	planCmd.SetContext(context.Background())

	if err := planCmd.RunE(planCmd, nil); err != nil {
		t.Fatalf("planCmd.RunE: %v", err)
	}
	got, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("read trace file: %v", err)
	}
	want := "plan -detailed-exitcode -var-file=../envs/prod/terraform.tfvars\n"
	if string(got) != want {
		t.Fatalf("plan args: want %q got %q", want, string(got))
	}
}

func TestApplyCommandAddsAutoApprove(t *testing.T) {
	d.log = newLogger("error")
	d.env = "prod"
	d.creds = rawCreds{Region: "us-east-1"}
	root := t.TempDir()
	stack := initRepoLayout(t, root, "prod")
	if err := os.MkdirAll(filepath.Join(stack, ".terraform"), 0o755); err != nil {
		t.Fatalf("mkdir .terraform: %v", err)
	}
	traceFile := filepath.Join(t.TempDir(), "trace.txt")
	t.Setenv("TRACE_FILE", traceFile)
	setupFakeTerraform(t, "printf '%s\n' "$*" > "$TRACE_FILE"")
	withWorkingDir(t, root)
	applyCmd.SetContext(context.Background())
	old := applyAutoApprove
	applyAutoApprove = true
	t.Cleanup(func() { applyAutoApprove = old })

	if err := applyCmd.RunE(applyCmd, nil); err != nil {
		t.Fatalf("applyCmd.RunE: %v", err)
	}
	got, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("read trace file: %v", err)
	}
	want := "apply -var-file=../envs/prod/terraform.tfvars -auto-approve\n"
	if string(got) != want {
		t.Fatalf("apply args: want %q got %q", want, string(got))
	}
}

func TestNukeCommandCancelsOnUnexpectedConfirmation(t *testing.T) {
	d.log = newLogger("error")
	d.env = "prod"
	d.creds = rawCreds{Region: "us-east-1"}
	root := t.TempDir()
	initRepoLayout(t, root, "prod")
	traceFile := filepath.Join(t.TempDir(), "trace.txt")
	t.Setenv("TRACE_FILE", traceFile)
	setupFakeTerraform(t, "printf '%s\n' "$*" > "$TRACE_FILE"")
	withWorkingDir(t, root)
	setStdinText(t, "nope\n")
	nukeCmd.SetContext(context.Background())

	if err := nukeCmd.RunE(nukeCmd, nil); err != nil {
		t.Fatalf("nukeCmd.RunE cancel path: %v", err)
	}
	if _, err := os.Stat(traceFile); !os.IsNotExist(err) {
		t.Fatalf("terraform should not run on cancel, stat err=%v", err)
	}
}

func TestNukeCommandRunsDestroyAfterConfirmation(t *testing.T) {
	d.log = newLogger("error")
	d.env = "prod"
	d.creds = rawCreds{Region: "us-east-1"}
	root := t.TempDir()
	stack := initRepoLayout(t, root, "prod")
	if err := os.MkdirAll(filepath.Join(stack, ".terraform"), 0o755); err != nil {
		t.Fatalf("mkdir .terraform: %v", err)
	}
	traceFile := filepath.Join(t.TempDir(), "trace.txt")
	t.Setenv("TRACE_FILE", traceFile)
	setupFakeTerraform(t, "printf '%s\n' "$*" > "$TRACE_FILE"")
	withWorkingDir(t, root)
	setStdinText(t, "destroy-prod\n")
	nukeCmd.SetContext(context.Background())

	if err := nukeCmd.RunE(nukeCmd, nil); err != nil {
		t.Fatalf("nukeCmd.RunE: %v", err)
	}
	got, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("read trace file: %v", err)
	}
	want := "destroy -var-file=../envs/prod/terraform.tfvars -auto-approve\n"
	if string(got) != want {
		t.Fatalf("destroy args: want %q got %q", want, string(got))
	}
}
