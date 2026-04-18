package version

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// AppVersion is the release label from the repo VERSION file, set at link time by the Makefile
// (-ldflags "-X ...version.AppVersion=..."). Empty means use runtime/debug build metadata only.
var AppVersion string

func shortVCSRevision(bi *debug.BuildInfo) string {
	if bi == nil {
		return ""
	}
	for _, s := range bi.Settings {
		if s.Key == "vcs.revision" && s.Value != "" {
			rev := s.Value
			if len(rev) > 12 {
				rev = rev[:12]
			}
			return rev
		}
	}
	return ""
}

// Line returns a single-line build label: VERSION-based when AppVersion is set, otherwise
// module path, semver or "(devel)", optional VCS revision.
func Line() string {
	if v := strings.TrimSpace(AppVersion); v != "" {
		bi, ok := debug.ReadBuildInfo()
		var rev string
		if ok {
			rev = shortVCSRevision(bi)
		}
		if rev != "" {
			return fmt.Sprintf("logsee %s (%s)", v, rev)
		}
		return fmt.Sprintf("logsee %s", v)
	}

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "logsee (build info unavailable)"
	}
	mod := bi.Main.Path
	if mod == "" {
		mod = "logsee"
	}
	ver := bi.Main.Version
	if ver == "" {
		ver = "unknown"
	}
	if rev := shortVCSRevision(bi); rev != "" {
		return fmt.Sprintf("%s %s (%s)", mod, ver, rev)
	}
	return fmt.Sprintf("%s %s", mod, ver)
}
