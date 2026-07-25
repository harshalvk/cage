package cmd

import (
	"fmt"
	"os"

	cageclient "github.com/harshalvk/cage/sdk/go"
)

func newClient() *cageclient.Client {
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "Error: no API key provided. Set --api-key of the CAGE_API_KEY environment variable.")
		os.Exit(1)
	}
	return cageclient.New(serverURL, apiKey)
}
