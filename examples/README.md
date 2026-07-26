# Examples

Minimal, runnable quickstarts showing the same flow (create a sandbox → run a command → clean up) in different languages/tools. Each is standalone — pick whichever matches your stack.

| | Language | Requires |
|---|---|---|
| [curl/](curl/) | Shell | Just `curl` |
| [go/](go/) | Go | Uses the [Go SDK](../sdk/go) |
| [python/](python/) | Python | `requests` |
| [typescript/](typescript/) | TypeScript | Node 18+ (native `fetch`) |

## Setup

All examples need a running Cage server and an API key:

```bash
# from the repo root, with the server running (see main README)
make genkey name=example
export CAGE_API_KEY=<the key make genkey prints>
```

Optionally set `CAGE_SERVER` if your server isn't on `http://localhost:8080`.

## Running

```bash
./curl/quickstart.sh
```

```bash
cd go && go run main.go
```

```bash
cd python && pip install -r requirements.txt && python3 quickstart.py
```

```bash
cd typescript && npm install && npm start
```

Want an example in another language? Open an issue or a PR — the pattern (create, exec, delete) is the same everywhere, just translate the three HTTP calls.
