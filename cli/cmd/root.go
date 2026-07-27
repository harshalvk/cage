package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	serverURL string
	apiKey    string
)

var rootCmd = &cobra.Command{
	Use:   "cage",
	Short: "Cage — manage isolated sandboxes from your terminal",
	Long:  "Cage is a CLI for creating, running commands in, and managing isolated Docker-backed sandboxes.",
}

var (
	buildVersion = "dev"
	buildCommit  = "none"
)

func SetVersionInfo(version, commit string) {
	buildVersion = version
	buildCommit = commit
	rootCmd.Version = fmt.Sprintf("%s (%s)", version, commit)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error: ", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&serverURL, "server", "", "Cage server URL")
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "Cage API key")
	rootCmd.PersistentPreRunE = resolveCredentials
}

func resolveCredentials(cmd *cobra.Command, args []string) error {
	// 1. Flags already win if explicitly set (Cobra leaves them as-is)
	// 2. Env vars, if flags weren't set
	if apiKey == "" {
		apiKey = os.Getenv("CAGE_API_KEY")
	}
	if serverURL == "" {
		serverURL = os.Getenv("CAGE_SERVER")
	}

	// 3. Config file, if still empty
	if apiKey == "" || serverURL == "" {
		cfg, err := loadConfig()
		if err == nil { // ignore config errors — just means nothing's saved yet
			if apiKey == "" {
				apiKey = cfg.APIKey
			}
			if serverURL == "" {
				serverURL = cfg.Server
			}
		}
	}

	// 4. Final default for server, if truly nothing was set anywhere
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	return nil
}
