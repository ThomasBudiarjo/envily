// Command envily is a small self-hosted secrets manager: one binary for the
// server, the web UI, and the CLI.
package main

import (
	"os"

	"github.com/thomasbudiarjo/envily/internal/cli"
)

func main() { os.Exit(cli.Main()) }
