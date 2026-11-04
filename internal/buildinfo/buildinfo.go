// Package buildinfo holds what the binary was built from, set at link time:
//
//	go build -ldflags "-X github.com/omareloui/tyche/internal/buildinfo.Commit=<sha>"
package buildinfo

// Commit is the git commit the binary was built from, "dev" when not set.
var Commit = "dev"
