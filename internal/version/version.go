// Package version holds build information set with -ldflags.
package version

import "fmt"

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func String() string {
	return fmt.Sprintf("envily %s (commit %s, built %s)", Version, Commit, Date)
}
