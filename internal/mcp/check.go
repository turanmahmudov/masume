package mcp

import (
	"context"
	"strconv"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db"
)

// What `--mcp --check` reports: every open profile, connected once. A fault in the setup is
// easier to find here than through the client of an agent.

// ProfileCheck is the result of the check of one profile.
type ProfileCheck struct {
	Name   string
	Target string
	Access cfg.McpAccess
	// TableCount is how many relations the connection read.
	TableCount int
	// Problem is empty where the connection opened and the relations were read.
	Problem string
}

// CheckOpenProfiles opens every profile an agent may reach, one at a time, so the failures are
// reported in the order of the file and a tunnel started for one profile can serve the next.
func CheckOpenProfiles(ctx context.Context, deps AccessDeps) []ProfileCheck {
	checks := []ProfileCheck{}

	for _, profile := range ListOpenProfiles(deps) {
		check := ProfileCheck{
			Name:   profile.Name,
			Target: cfg.DescribeProfileTarget(profile),
			Access: ResolveProfileAccess(deps.Config, profile),
		}
		if unreachable := FindUnreachableReason(profile); unreachable != "" {
			check.Problem = unreachable
			checks = append(checks, check)
			continue
		}
		connection, err := OpenNamedConnection(ctx, deps, profile)
		if err != nil {
			check.Problem = db.DescribeError(err)
			checks = append(checks, check)
			continue
		}
		check.TableCount = len(connection.Tables())
		checks = append(checks, check)
	}

	return checks
}

// DescribeCheck writes one line per profile, with the fault where there is one.
func DescribeCheck(check ProfileCheck) string {
	head := check.Name + " (" + string(check.Access) + ") " + check.Target
	if check.Problem != "" {
		return "FAILED  " + head + "\n        " + check.Problem
	}
	return "ok      " + head + " · " + strconv.Itoa(check.TableCount) + " tables"
}
