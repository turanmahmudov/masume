package cli

import "runtime/debug"

// Version is the release version, set with -ldflags "-X .../internal/cli.Version=...".
// Development builds use the module version or the recorded revision.
var Version = "dev"

// ResolveVersion returns the release version, the module version, or the recorded revision.
func ResolveVersion() string {
	if Version != "dev" {
		return Version
	}
	info, read := debug.ReadBuildInfo()
	if !read {
		return Version
	}
	revision, modified := "", false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	return describeBuildVersion(Version, info.Main.Version, revision, modified)
}

// describeBuildVersion picks the version a build records for itself. A module install
// stamps the module version. A source build has no version, so the recorded revision
// stands in for it.
func describeBuildVersion(releaseVersion, mainVersion, revision string, modified bool) string {
	if mainVersion != "" && mainVersion != "(devel)" {
		return mainVersion
	}
	if revision == "" {
		return releaseVersion
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if modified {
		return releaseVersion + "+" + revision + "-dirty"
	}
	return releaseVersion + "+" + revision
}
