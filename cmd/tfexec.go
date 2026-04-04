package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// stackDir and envsDir define the terraform project layout for this repo.
const (
	stackDirName = "terraform/stack"
	envsDirName  = "terraform/envs"
)

// repoRoot returns the absolute path to the repository root, resolved via
// `git rev-parse --show-toplevel` so the CLI works correctly when invoked
// from any subdirectory of the repository.
func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		// Fall back to the working directory if git is unavailable.
		return filepath.Abs(".")
	}
	return strings.TrimSpace(string(out)), nil
}

// stackDir returns the absolute path to the terraform stack directory.
func stackDir() (string, error) {
	root, err := repoRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, stackDirName), nil
}

// backendArgs builds the -backend-config flags for terraform init.
// If terraform/stack/backend.local.hcl exists (gitignored, contains the real
// bucket/table/region), it is prepended before the env backend config. The
// env-specific terraform/envs/<env>/backend.hcl is passed after and supplies
// the per-environment state key. Later flags take precedence in Terraform, so
// the env key always wins over any key in the local override file.
func backendArgs(stackPath, root, env string) []string {
	envsBackend := filepath.Join(root, envsDirName, env, "backend.hcl")
	// Paths are relative to the stack dir since terraform runs there.
	relEnvs, err := filepath.Rel(stackPath, envsBackend)
	if err != nil {
		relEnvs = filepath.Join("..", envsDirName, env, "backend.hcl")
	}

	args := []string{"-backend-config=" + relEnvs}

	local := filepath.Join(stackPath, "backend.local.hcl")
	if _, err := os.Stat(local); err == nil {
		args = append([]string{"-backend-config=backend.local.hcl"}, args...)
	}

	return args
}

// varFileArg returns the -var-file flag for terraform plan/apply/destroy.
func varFileArg(stackPath, root, env string) string {
	tfvars := filepath.Join(root, envsDirName, env, "terraform.tfvars")
	rel, err := filepath.Rel(stackPath, tfvars)
	if err != nil {
		rel = filepath.Join("..", envsDirName, env, "terraform.tfvars")
	}
	return "-var-file=" + rel
}

// isInitialised reports whether terraform has already been initialised in
// the given stack directory (i.e. .terraform/ exists).
func isInitialised(stackPath string) bool {
	_, err := os.Stat(filepath.Join(stackPath, ".terraform"))
	return err == nil
}

// runOptions configures a terraform subprocess invocation.
type runOptions struct {
	stackPath string
	args      []string
	creds     rawCreds
	stdin     io.Reader // nil for non-interactive; os.Stdin for apply prompts
	stdout    io.Writer // defaults to os.Stdout
	stderr    io.Writer // defaults to os.Stderr
}

// runTerraform executes terraform with the given args in stackPath, streaming
// output to the terminal. Returns terraform's exit code directly.
//
// Exit code 2 from plan means "changes present" — callers must treat this as
// a non-error condition.
func runTerraform(ctx context.Context, opts runOptions) (int, error) {
	if opts.stdout == nil {
		opts.stdout = os.Stdout
	}
	if opts.stderr == nil {
		opts.stderr = os.Stderr
	}

	cmd := exec.CommandContext(ctx, "terraform", opts.args...) //nolint:gosec
	cmd.Dir = opts.stackPath

	// Build subprocess env: parent env + injected AWS creds.
	env := os.Environ()
	for k, v := range opts.creds.toEnv() {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	cmd.Stdin = opts.stdin
	cmd.Stdout = opts.stdout
	cmd.Stderr = opts.stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return -1, fmt.Errorf("running terraform: %w", err)
	}
	return 0, nil
}

// terraformInit runs terraform init in the given stack directory.
// It is called automatically before plan/apply/destroy when .terraform/ is absent.
func terraformInit(ctx context.Context, stackPath, root, env string, creds rawCreds) error {
	args := append([]string{"init"}, backendArgs(stackPath, root, env)...)
	code, err := runTerraform(ctx, runOptions{
		stackPath: stackPath,
		args:      args,
		creds:     creds,
	})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("terraform init exited with code %d", code)
	}
	return nil
}

// ensureInit runs terraform init if the stack has not been initialised yet.
func ensureInit(ctx context.Context, stackPath, root, env string, creds rawCreds) error {
	if isInitialised(stackPath) {
		return nil
	}
	d.log.Info("stack not initialised; running terraform init", "stack", stackPath)
	return terraformInit(ctx, stackPath, root, env, creds)
}
