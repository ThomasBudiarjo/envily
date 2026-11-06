//go:build !windows

package cli

import "syscall"

// execProcess replaces the CLI with the command, so signals and the exit
// status go to and come from the command directly.
func execProcess(path string, argv, env []string) error {
	return syscall.Exec(path, argv, env) //nolint:gosec // running the user's command is the point
}
