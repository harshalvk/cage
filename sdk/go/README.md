# cageclient — Go SDK for Cage

Official Go client for [Cage](https://github.com/harshalvk/cage).

## Install

```bash
go get github.com/harshalvk/cage/sdk/go
```
## Usage

```go
import cageclient "github.com/harshalvk/cage/sdk/go"

client := cageclient.New("http://localhost:8080", "your-api-key")

sb, err := client.CreateSandbox(ctx, cageclient.CreateSandboxOptions{Template: "python-3.12"})
result, err := client.Exec(ctx, sb.ID, []string{"python3", "-c", "print('hello')"})
fmt.Println(result.Stdout)
```

See [example_test.go](example_test.go) for a full runnable example.

## Options

```go
client := cageclient.New(baseURL, apiKey,
    cageclient.WithTimeout(60 * time.Second),
)
```