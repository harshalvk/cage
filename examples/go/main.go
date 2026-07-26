package main

import (
	"context"
	"fmt"
	"log"
	"os"

	cageclient "github.com/harshalvk/cage/sdk/go"
)

func main() {
	server := os.Getenv("CAGE_SERVER")
	if server == "" {
		server = "http://localhost:8080"
	}
	apiKey := os.Getenv("CAGE_API_KEY")
	if apiKey == "" {
		log.Fatal("set CAGE_API_KEY (generate one with 'make genkey' on the server)")
	}

	client := cageclient.New(server, apiKey)
	ctx := context.Background()

	fmt.Println("→ Creating a Node sandbox...")
	sb, err := client.CreateSandbox(ctx, cageclient.CreateSandboxOptions{Template: "node-20"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("  created:", sb.ID)

	fmt.Println("→ Running a command inside it...")
	result, err := client.Exec(ctx, sb.ID, []string{"node", "-e", "console.log(2 + 2)"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(result.Stdout)

	fmt.Println("→ Cleaning up...")
	if err := client.DeleteSandbox(ctx, sb.ID); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Done.")
}
