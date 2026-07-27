package main

import "github.com/harshalvk/cage/cli/cmd"

var (
	version = "dev"
	commit  = "none"
)

func main() {
	cmd.SetVersionInfo(version, commit)
	cmd.Execute()
}
