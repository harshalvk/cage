package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	cageclient "github.com/harshalvk/cage/sdk/go"
	"github.com/spf13/cobra"
)

var sandboxCmd = &cobra.Command{
	Use:   "sandbox",
	Short: "Manage sandboxes",
}

var template string

var sandboxCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new sandbox",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newClient()
		sb, err := client.CreateSandbox(context.Background(), cageclient.CreateSandboxOptions{Template: template})
		if err != nil {
			return err
		}
		fmt.Printf("Created sandbox %s (template: %s, status: %s)\n", sb.ID, sb.TemplateSlug, sb.Status)
		return nil
	},
}

var sandboxListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List sandboxes",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newClient()
		sandboxes, err := client.ListSandboxes(context.Background())
		if err != nil {
			return err
		}

		if len(sandboxes) == 0 {
			fmt.Println("No sandboxes.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTEMPLATE\tSTATUS\tEXPIRES")
		for _, sb := range sandboxes {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", sb.ID, sb.TemplateSlug, sb.Status, sb.ExpiresAt.Format(time.RFC3339))
		}
		return w.Flush()
	},
}

var sandboxGetCmd = &cobra.Command{
	Use:   "get [sandbox-id]",
	Short: "Get sandbox details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newClient()
		sb, err := client.GetSandbox(context.Background(), args[0])
		if err != nil {
			return err
		}
		fmt.Printf("ID:      %s\nTemplate: %s\nStatus:  %s\nCreated:  %s\nExpires:  %s\n", sb.ID, sb.TemplateSlug, sb.Status, sb.CreatedAt.Format(time.RFC3339), sb.ExpiresAt.Format(time.RFC3339))
		return nil
	},
}

var sandboxRmCmd = &cobra.Command{
	Use:   "rm [sandbox-id]",
	Short: "Delete a sandbox",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newClient()
		if err := client.DeleteSandbox(context.Background(), args[0]); err != nil {
			return err
		}
		fmt.Println("Deleted.")
		return nil
	},
}

var sandboxPauseCmd = &cobra.Command{
	Use:   "pause [sandbox-id]",
	Short: "Pause a running sandbox",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newClient()
		sb, err := client.PauseSandbox(context.Background(), args[0])
		if err != nil {
			return err
		}
		fmt.Printf("Sandbox %s is now %s\n", sb.ID, sb.Status)
		return nil
	},
}

var sandboxResumeCmd = &cobra.Command{
	Use:   "resume [sandbox-id]",
	Short: "Resume a paused sandbox",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := newClient()
		sb, err := client.ResumeSandbox(context.Background(), args[0])
		if err != nil {
			return err
		}
		fmt.Printf("Sandbox %s is now %s\n", sb.ID, sb.Status)
		return nil
	},
}

func init() {
	sandboxCreateCmd.Flags().StringVarP(&template, "template", "t", "base", "Template slug to use")

	sandboxCmd.AddCommand(sandboxCreateCmd, sandboxListCmd, sandboxGetCmd, sandboxRmCmd, sandboxPauseCmd, sandboxResumeCmd)
	rootCmd.AddCommand(sandboxCmd)
}
