module github.com/harshalvk/cage/cli

go 1.26.5

require (
	github.com/harshalvk/cage/sdk/go v0.0.0-00010101000000-000000000000
	github.com/spf13/cobra v1.10.2
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
)

replace github.com/harshalvk/cage/sdk/go => ../sdk/go
