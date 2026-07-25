package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec [sandbox-id] -- [command...]",
	Short: "Run a command inside a sandbox",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sandboxID := args[0]
		command := args[1:]

		client := newClient()
		result, err := client.Exec(context.Background(), sandboxID, command)
		if err != nil {
			return err
		}

		if result.Stdout != "" {
			fmt.Print(result.Stdout)
		}
		if result.Stderr != "" {
			fmt.Fprint(os.Stderr, result.Stderr)
		}

		if result.ExitCode != 0 {
			os.Exit(result.ExitCode) // propagate the sandbox's own exit code, like a real shell would
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
}
