package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Save your API key and server URL for future commands",
	RunE: func(cmd *cobra.Command, args []string) error {
		if apiKey == "" {
			return fmt.Errorf("provide --api-key to save")
		}

		cfg := &CLIConfig{Server: serverURL, APIKey: apiKey}
		if err := saveConfig(cfg); err != nil {
			return err
		}

		path, _ := configPath()
		fmt.Printf("Saved credentials to %s\n", path)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)
}
