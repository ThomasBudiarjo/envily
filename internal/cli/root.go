// Package cli implements the envily command line: the server, local admin
// commands, and client commands that talk to a server over HTTP.
package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
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
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Getenv  func(string) (string, bool)
	Environ func() []string
	Getwd   func() (string, error)
	// ReadPassword reads a line without echo; nil reads a plain line.
	ReadPassword func() (string, error)
	HTTPClient   *http.Client
	LookPath     func(string) (string, error)
	// Exec replaces the process with path (or runs it and exits with its
	// status where exec is unavailable). It returns only on failure.
	Exec func(path string, argv, env []string) error

	in *bufio.Reader
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

	addClientFlags(root)
	root.AddCommand(
		newLoginCmd(app),
		newLogoutCmd(app),
		newWhoamiCmd(app),
		newLinkCmd(app),
		newProjectsCmd(app),
		newEnvsCmd(app),
		newSecretsCmd(app),
		newRunCmd(app),
		newExportCmd(app),
		newImportCmd(app),
		newServerCmd(app),
		newVersionCmd(app),
		newKeygenCmd(app),
		newMigrateCmd(app),
		newBackupCmd(app),
		newAdminCmd(app),
	)
	usageArgs(root)
	return root
}

// usageArgs makes argument-count errors exit with ExitUsage.
func usageArgs(cmd *cobra.Command) {
	if check := cmd.Args; check != nil {
		cmd.Args = func(c *cobra.Command, args []string) error {
			if err := check(c, args); err != nil {
				return withCode(ExitUsage, err)
			}
			return nil
		}
	}
	for _, sub := range cmd.Commands() {
		usageArgs(sub)
	}
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
	if strings.HasPrefix(err.Error(), "unknown command") {
		return ExitUsage
	}
	return ExitError
}

// Main runs the CLI with the real process environment.
func Main() int {
	app := &App{
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		Getenv: os.LookupEnv, Environ: os.Environ, Getwd: os.Getwd,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
		LookPath:   exec.LookPath, Exec: execProcess,
	}
	if term.IsTerminal(int(os.Stdin.Fd())) { //nolint:gosec // fd fits in int
		app.ReadPassword = func() (string, error) {
			b, err := term.ReadPassword(int(os.Stdin.Fd())) //nolint:gosec // fd fits in int
			return string(b), err
		}
	}
	return Run(context.Background(), app, os.Args[1:])
}
