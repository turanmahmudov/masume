package cli

import "testing"

// The version a build reports follows the install method: a module install carries the
// module version, a source build falls back to the recorded revision.
func TestBuildVersionFollowsTheInstallMethod(t *testing.T) {
	for _, one := range []struct {
		releaseVersion string
		mainVersion    string
		revision       string
		modified       bool
		want           string
	}{
		{"dev", "v0.0.1", "", false, "v0.0.1"},
		{"dev", "v0.0.1", "abc123def456", false, "v0.0.1"},
		{"dev", "(devel)", "abc123def456ab", false, "dev+abc123def456"},
		{"dev", "(devel)", "abc123def456ab", true, "dev+abc123def456-dirty"},
		{"dev", "(devel)", "", false, "dev"},
		{"dev", "", "abc123def456ab", false, "dev+abc123def456"},
	} {
		got := describeBuildVersion(one.releaseVersion, one.mainVersion, one.revision, one.modified)
		if got != one.want {
			t.Errorf("describeBuildVersion(%q, %q, %q, %v) returns %q, wanted %q",
				one.releaseVersion, one.mainVersion, one.revision, one.modified, got, one.want)
		}
	}
}
