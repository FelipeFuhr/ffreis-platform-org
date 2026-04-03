package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var applyAutoApprove bool

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Run terraform apply for the given environment",
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

		if err := ensureInit(ctx, stack, root, d.env, d.creds); err != nil {
			return fmt.Errorf("terraform init: %w", err)
		}

		d.log.Info("running terraform apply", "env", d.env, "auto_approve", applyAutoApprove)

		args := []string{"apply", varFileArg(stack, root, d.env)}
		if applyAutoApprove {
			args = append(args, "-auto-approve")
		}

		// Pass stdin through so the user can respond to terraform's prompt.
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
			return fmt.Errorf("terraform apply exited with code %d", code)
		}

		d.log.Info("apply complete")
		return nil
	},
}

func init() {
	applyCmd.Flags().BoolVar(&applyAutoApprove, "auto-approve", false, "Skip interactive approval")
	rootCmd.AddCommand(applyCmd)
}
