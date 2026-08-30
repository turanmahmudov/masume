package mongo

import (
	"errors"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

func buildProbeProfile(user string) cfg.Profile {
	return cfg.Profile{
		Name: "orders", Engine: core.EngineMongo, Host: "cluster.example.com",
		Port: 27017, Database: "shop", User: user,
	}
}

// The driver reports that it chose no server and writes the whole topology after it. The
// reason sits inside that, and the reason is what the user has to act on.
func TestDescribeConnectFailureAnswersTheReasonAndNotTheTopology(t *testing.T) {
	err := errors.New("server selection error: context deadline exceeded, " +
		"current topology: { Type: Unknown, Servers: [{ Addr: cluster.example.com:27017, " +
		"Type: Unknown, Last error: tls: failed to verify certificate: x509: " +
		"certificate signed by unknown authority }, ] }")

	written := DescribeConnectFailure(buildProbeProfile("reader"), err)
	if !strings.HasSuffix(written, "x509: certificate signed by unknown authority") {
		t.Errorf("the message reads %q", written)
	}
	if strings.Contains(written, "topology") {
		t.Errorf("the message carries the whole topology:\n%s", written)
	}
	if !strings.Contains(written, "cluster.example.com") {
		t.Errorf("the message does not name the server:\n%s", written)
	}
}

// An error that carries no topology is written as it stands.
func TestDescribeConnectFailureKeepsAnErrorItCannotShorten(t *testing.T) {
	written := DescribeConnectFailure(buildProbeProfile("reader"), errors.New("no route to host"))
	if !strings.Contains(written, "no route to host") {
		t.Errorf("the message reads %q", written)
	}
}

// A server that authenticates every command refuses one that is not authenticated. Where
// the profile names no user, that is the reason, and the server does not say it.
func TestBuildAuthenticationMessageNamesTheMissingUser(t *testing.T) {
	err := errors.New("(Unauthorized) Command buildInfo requires authentication")

	written := BuildAuthenticationMessage(buildProbeProfile(""), err)
	if !strings.Contains(written, "the server needs a user and this profile names none") {
		t.Errorf("the message reads %q", written)
	}
	// A profile that does name a user is told what the server said.
	written = BuildAuthenticationMessage(buildProbeProfile("reader"), err)
	if !strings.Contains(written, "requires authentication") {
		t.Errorf("the message reads %q", written)
	}
}

// Only the codes that say who the connection is count as an authentication failure. Any
// other refusal leaves the connection open, because the server answered it.
func TestIsAuthenticationErrorNamesOnlyTheCodesOfWhoTheConnectionIs(t *testing.T) {
	for _, held := range []struct {
		code int32
		want bool
	}{
		{13, true},  // Unauthorized
		{18, true},  // AuthenticationFailed
		{26, false}, // NamespaceNotFound
		{59, false}, // CommandNotFound
	} {
		err := mongo.CommandError{Code: held.code, Message: "reported"}
		if answered := IsAuthenticationError(err); answered != held.want {
			t.Errorf("code %d reads as %v, wanted %v", held.code, answered, held.want)
		}
	}
	if IsAuthenticationError(errors.New("no route to host")) {
		t.Error("an error the server never sent reads as one about who the connection is")
	}
}
