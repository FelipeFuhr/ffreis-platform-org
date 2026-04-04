package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var nukeForce = true

var nukeCmd = &cobra.Command{
	Use:   "nuke",
	Short: "Destroy all resources in the given environment (IRREVERSIBLE)",
	Long: `nuke runs terraform destroy after an explicit confirmation prompt.

NOTE: The state bucket and lock table in this stack have force_destroy = false.
Empty those S3 buckets before running nuke, or the destroy will fail.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()

		root, err := repoRoot()
		if err != nil {
			return err
		}
		stack, err := stackDir()
		if err != nil {
			return err
		}

		confirm := "destroy-" + d.env
		_, _ = io.WriteString(os.Stderr, "\nWARNING: This will permanently destroy all resources in env "+strconv.Quote(d.env)+".\n")
		_, _ = io.WriteString(os.Stderr, "The runtime S3 bucket and DynamoDB lock table have force_destroy = false.\n")
		_, _ = io.WriteString(os.Stderr, "Empty those S3 buckets before proceeding, or the destroy will fail.\n\n")
		_, _ = io.WriteString(os.Stderr, "Type "+strconv.Quote(confirm)+" to confirm: ")

		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return fmt.Errorf("no input received")
		}
		if strings.TrimSpace(scanner.Text()) != confirm {
			_, _ = io.WriteString(os.Stderr, "Cancelled.\n")
			return nil
		}

		if err := ensureInit(ctx, stack, root, d.env, d.creds); err != nil {
			return fmt.Errorf("terraform init: %w", err)
		}

		d.log.Info("running terraform destroy", "env", d.env, "force", nukeForce)

		args := append([]string{"destroy"}, varFileArgs(stack, root, d.env)...)
		if nukeForce {
			args = append(args, "-auto-approve")
		}
		code, err := runTerraform(ctx, runOptions{
			stackPath: stack,
			args:      args,
			creds:     d.creds,
			stdin:     os.Stdin,
		})
		if err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("terraform destroy exited with code %d", code)
		}

		d.log.Info("nuke complete")
		return nil
	},
}

func init() {
	nukeCmd.Flags().BoolVar(&nukeForce, "force", true, "Skip Terraform's final approval prompt")
	rootCmd.AddCommand(nukeCmd)
}
