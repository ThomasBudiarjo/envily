// Package cli implements the envily command line: the server, local admin
// commands, and client commands that talk to a server over HTTP.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// Exit codes.
const (
	ExitOK           = 0
	ExitError        = 1
	ExitUsage        = 2
	ExitUnauthorized = 3
	ExitForbidden    = 4
	ExitNotFound     = 5
)

// exitError carries a specific exit code out of a command.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

func withCode(code int, err error) error { return &exitError{code: code, err: err} }

// App holds the I/O streams and process environment so commands can be
// run in tests.
type App struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Getenv func(string) (string, bool)
}

func newRoot(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:           "envily",
		Short:         "Envily is a small self-hosted secrets manager",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetIn(app.Stdin)
	root.SetOut(app.Stdout)
	root.SetErr(app.Stderr)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return withCode(ExitUsage, err) })

	root.AddCommand(
		newServerCmd(app),
		newVersionCmd(app),
	)
	return root
}

// Run executes the command line and returns the process exit code.
func Run(ctx context.Context, app *App, args []string) int {
	root := newRoot(app)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return ExitOK
	}
	fmt.Fprintln(app.Stderr, "error:", err)
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	return ExitError
}

// Main runs the CLI with the real process environment.
func Main() int {
	return Run(context.Background(), &App{
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Getenv: os.LookupEnv,
	}, os.Args[1:])
}
