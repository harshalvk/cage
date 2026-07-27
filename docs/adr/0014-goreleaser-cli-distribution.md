# 0014.SDK and CLI as separate Go modules within the same repo

Date: 2026-07-19

## Status

Accepted

## Context

Requiring `go install`/`go build` to use the CLI is a real adoption barrier — it assumes the user already has Go installed and is comfortable with its tooling, which is not a reasonable assumption for a CLI meant to be broadly usable. Real-world CLIs (`gh`, `kubectl`, `docker`) distribute pre-built binaries through OS-native package managers, so installation is a single familiar command regardless of the user's development background.

Cross-compiling manually (running `GOOS=... GOARCH=... go build` for every OS/architecture combination, packaging archives, computing checksums, drafting GitHub Releases, and writing/maintaining Homebrew formula and Scoop manifest files by hand) is mechanical, repetitive, and easy to get subtly wrong or let drift out of sync with actual releases. GoReleaser automates this entire pipeline from a single declarative config file, and is the de facto standard for exactly this problem in the Go ecosystem.

A separate design question was which platforms to support. The developer's own environment is Windows, where Homebrew doesn't apply — Scoop is the closest equivalent (a package manager with a similar "bucket" plugin model). Supporting only Homebrew would have left the primary development platform unable to use its own distribution mechanism, which is both a real gap for any Windows user and would have delayed noticing packaging bugs specific to that platform.

## Decision

Use GoReleaser, triggered by CI on `cli/v*` tags (a prefixed tag scheme, since the SDK and server may need independent versioning later), to build cross-platform binaries and publish them as a GitHub Release, a Homebrew tap (`harshalvk/homebrew-cage`), and a Scoop bucket (`harshalvk/scoop-cage`) — each a separate GitHub repository per that ecosystem's convention, populated automatically by GoReleaser rather than hand-maintained.

## Consequences

- Installation becomes `brew install harshalvk/cage/cage` (macOS/Linux) or `scoop bucket add` + `scoop install` (Windows), matching the idioms users of each platform already know, with a curl-based install script as a fallback for environments with neither package manager.
- Both tap/bucket repos require their own scoped GitHub tokens (`HOMEBREW_TAP_GITHUB_TOKEN`, `SCOOP_BUCKET_GITHUB_TOKEN`) stored as secrets on the main repo — this is additional credential surface to maintain and rotate compared to a single-repo release process, though each token is scoped narrowly to only its respective tap/bucket repo.
- The `cli/v*` tag prefix (rather than a bare `v*`) commits to a versioning scheme where the CLI, SDK, and server can eventually be tagged and released independently of one another. This adds a small amount of tagging discipline (remembering the prefix) in exchange for avoiding ambiguity later about what a given version number refers to.
- The fallback install script (`install.sh`) uses an unauthenticated `curl | bash` pattern, which is a known supply-chain trust concern — the script currently does not verify the downloaded binary against the checksums GoReleaser already publishes. Adding checksum verification to the script is a known, not-yet-addressed follow-up before treating it as a fully trustworthy distribution path.
