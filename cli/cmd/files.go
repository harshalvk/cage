package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var filePushCmd = &cobra.Command{
	Use:   "push [sandbox-id] [local-file] [remote-path]",
	Short: "Upload a local file into a sandbox",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		content, err := os.ReadFile(args[1])
		if err != nil {
			return fmt.Errorf("failed to read local file: %w", err)
		}

		client := newClient()
		if err := client.WriteFile(context.Background(), args[0], args[2], string(content)); err != nil {
			return err
		}
		fmt.Printf("Uploaded %s to %s:%s\n", args[1], args[0], args[2])
		return nil
	},
}

var filePullCmd = &cobra.Command{
	Use:   "pull [sandbox-id] [remote-path] [local-file]",
	Short: "Download a file from a sandbox",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newClient()
		content, err := client.ReadFile(context.Background(), args[0], args[1])
		if err != nil {
			return err
		}

		if err := os.WriteFile(args[2], content, 0644); err != nil {
			return fmt.Errorf("failed to write local file: %w", err)
		}
		fmt.Printf("Downloaded %s:%s to %s\n", args[0], args[1], args[2])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(filePushCmd, filePullCmd)
}
