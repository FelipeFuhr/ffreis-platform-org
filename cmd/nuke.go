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

var nukeCmd = &cobra.Command{
	Use:   "nuke",
	Short: "Destroy all resources in the given environment (IRREVERSIBLE)",
	Long: `nuke runs terraform destroy -auto-approve after an explicit confirmation prompt.

NOTE: The state bucket and lock table in this stack have prevent_destroy = true.
Remove those lifecycle blocks from terraform/stack/state.tf before running nuke, or the
destroy will fail at plan time.`,
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
		_, _ = io.WriteString(os.Stderr, "The runtime S3 bucket and DynamoDB lock table have prevent_destroy = true.\n")
		_, _ = io.WriteString(os.Stderr, "Remove those lifecycle blocks from terraform/stack/state.tf before proceeding.\n\n")
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

		d.log.Info("running terraform destroy", "env", d.env)

		args := []string{"destroy", varFileArg(stack, root, d.env), "-auto-approve"}
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
	rootCmd.AddCommand(nukeCmd)
}
