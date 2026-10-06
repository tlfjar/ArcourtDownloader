// Package buildinfo contains the identity embedded in both application binaries.
// Build scripts set these variables with Go linker flags; ordinary go builds stay
// explicitly identifiable as development builds.
package buildinfo

import (
	"fmt"
	"strings"
)

var Version = "development"
var Commit = "unknown"
var Release = "false"

// Record is also readable without executing the PE file. The release verifier
// checks this linker-stamped record alongside Go build info and Authenticode.
var Record string

type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Release bool   `json:"release"`
}

func Current() Info {
	if fields := strings.Split(Record, "|"); len(fields) == 6 && fields[0] == "ARCOURT_BUILD_V1" && fields[5] == "END_ARCOURT_BUILD" {
		return Info{Version: fields[1], Commit: fields[2], Release: fields[3] == "true"}
	}
	return Info{Version: Version, Commit: Commit, Release: Release == "true"}
}

func (i Info) String() string {
	if i.Release {
		return fmt.Sprintf("Arcourt Downloader %s (source %s)", i.Version, i.Commit)
	}
	return fmt.Sprintf("Arcourt Downloader %s (source %s; development, unsigned)", i.Version, i.Commit)
}
