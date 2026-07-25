package cageclient_test

import (
	"context"
	"fmt"

	cageclient "github.com/harshalvk/cage/sdk/go"
)

// This example demonstrates typical usage. It is not run by `go test` since
// it requires a live Cage server — see client_test.go for real, verified
// unit tests against a mocked server.
func Example() {
	client := cageclient.New("http://localhost:8080", "cage_4e944a50777d9ccaf9d7e7745aedf167b9c6ed8ff1d5d6a7a793bbd5fcbe4c16")
	ctx := context.Background()

	sb, err := client.CreateSandbox(ctx, cageclient.CreateSandboxOptions{Template: "python-3.12"})
	if err != nil {
		panic(err)
	}
	fmt.Println("created sandbox:", sb.ID)
}
