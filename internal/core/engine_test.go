package core

import "testing"

// Every engine in the list needs a registry entry, or it cannot be opened. This test walks
// the whole list, so a new engine without an entry fails here and not when a user selects
// it.
func TestEveryEngineHasAnEntry(t *testing.T) {
	if len(Engines) == 0 {
		t.Fatal("the client offers no engine")
	}
	for _, engine := range Engines {
		info := ResolveEngineInfo(engine)
		if info.Engine != engine {
			t.Errorf("%q answers an entry for %q", engine, info.Engine)
			continue
		}
		if info.Family == "" {
			t.Errorf("%q belongs to no family", engine)
		}
		// A server is reached over a port. A file is opened by path and needs no port.
		if info.OpensFile {
			if info.DefaultPort != 0 {
				t.Errorf("%q opens a file and still names port %d", engine, info.DefaultPort)
			}
			continue
		}
		if info.DefaultPort <= 0 {
			t.Errorf("%q reaches a server and names no port", engine)
		}
		// A hosted engine has no URL scheme of its own. It uses the scheme of its
		// protocol, which belongs to the base engine of the family.
	}
}

func TestListEngineInfoAnswersEveryEngineInOrder(t *testing.T) {
	listed := ListEngineInfo()
	if len(listed) != len(Engines) {
		t.Fatalf("the list holds %d entries and there are %d engines", len(listed), len(Engines))
	}
	for at, info := range listed {
		if info.Engine != Engines[at] {
			t.Errorf("entry %d is %q, wanted %q", at, info.Engine, Engines[at])
		}
	}
}

func TestFindEngineReadsTheNameAndRefusesTheRest(t *testing.T) {
	for _, engine := range Engines {
		if found, is := FindEngine(string(engine)); !is || found != engine {
			t.Errorf("%q was not found by its own name", engine)
		}
	}
	for _, written := range []string{"", "  ", "postgre", "my sql", "oracle"} {
		if _, is := FindEngine(written); is {
			t.Errorf("%q was read as an engine", written)
		}
	}
}

// Every family that connects to a server has a URL scheme, so a user can paste a URL for
// one of its engines. A family that opens a file uses a path and needs no scheme.
func TestEveryServerFamilyOwnsAUrlScheme(t *testing.T) {
	owned := map[Family]int{}
	for _, info := range ListEngineInfo() {
		if info.OpensFile {
			continue
		}
		owned[info.Family] += len(info.URLSchemes)
	}
	for family, count := range owned {
		if count == 0 {
			t.Errorf("no engine of the %q family answers a URL scheme", family)
		}
	}
}

// A URL scheme belongs to one engine only, or a pasted URL would open the wrong server.
func TestNoTwoEnginesClaimOneUrlScheme(t *testing.T) {
	owner := map[string]Engine{}
	for _, info := range ListEngineInfo() {
		for _, scheme := range info.URLSchemes {
			if held, taken := owner[scheme]; taken {
				t.Errorf("%q is claimed by %q and by %q", scheme, held, info.Engine)
				continue
			}
			owner[scheme] = info.Engine
		}
	}
}

// A capability the engine does not have must stay false, because the related keys are left
// unbound instead of showing an error.
func TestCapabilitiesFollowTheFamily(t *testing.T) {
	for _, held := range []struct {
		engine Engine
		field  string
		got    bool
		want   bool
	}{
		{EnginePostgres, "plans a statement", ResolveEngineInfo(EnginePostgres).Capabilities.PlansStatement, true},
		{EnginePostgres, "has transactions", ResolveEngineInfo(EnginePostgres).Capabilities.HasTransactions, true},
		{EngineMysql, "has transactions", ResolveEngineInfo(EngineMysql).Capabilities.HasTransactions, true},

		// A key store uses its own order and has no transaction the client can control.

		// A file has no server sessions to list or to cancel.
		{EngineSqlite, "has server sessions", ResolveEngineInfo(EngineSqlite).Capabilities.HasServerSessions, false},
		{EngineSqlite, "cancels a running query", ResolveEngineInfo(EngineSqlite).Capabilities.CancelsRunningQuery, false},
	} {
		if held.got != held.want {
			t.Errorf("%q %s reads %v, wanted %v", held.engine, held.field, held.got, held.want)
		}
	}
}

// A cancel needs a second connection to the same server, so an engine that reports the
// capability must also allow a second connection.
func TestOnlyAServerCancelsARunningQuery(t *testing.T) {
	for _, info := range ListEngineInfo() {
		if info.Capabilities.CancelsRunningQuery && info.OpensFile {
			t.Errorf("%q opens a file and reports that it cancels a running query", info.Engine)
		}
	}
}

func TestHoldsSystemSchemaMatchesTheNamesAndThePrefixes(t *testing.T) {
	postgres := ResolveEngineInfo(EnginePostgres)
	for _, held := range []struct {
		schema string
		want   bool
	}{
		{"pg_catalog", true},
		{"information_schema", true},
		// A prefix covers the schemas a server creates for itself, such as pg_toast_temp_1.
		{"pg_toast", true},
		{"public", false},
		{"shop", false},
		{"", false},
	} {
		if answered := postgres.HoldsSystemSchema(held.schema); answered != held.want {
			t.Errorf("postgres reads %q as system=%v, wanted %v", held.schema, answered, held.want)
		}
	}
}

func TestOpensFileAndNeedsUserAgreeWithTheEntry(t *testing.T) {
	for _, info := range ListEngineInfo() {
		if OpensFile(info.Engine) != info.OpensFile {
			t.Errorf("%q disagrees with its entry about opening a file", info.Engine)
		}
		if NeedsUser(info.Engine) != info.NeedsUser {
			t.Errorf("%q disagrees with its entry about needing a user", info.Engine)
		}
		if ResolveDefaultPort(info.Engine) != info.DefaultPort {
			t.Errorf("%q disagrees with its entry about its port", info.Engine)
		}
	}
}

// An unknown engine gives the entry of the default engine, so a caller never reads a port
// of zero or an empty family. The engine name is validated when a profile is built. This
// behaviour is the fallback, not the check for a bad name.
func TestAnUnknownEngineAnswersTheDefaultEntry(t *testing.T) {
	held := ResolveEngineInfo(Engine("oracle"))
	wanted := ResolveEngineInfo(DefaultEngine)
	if held.Engine != wanted.Engine {
		t.Errorf("an unknown engine answered the entry of %q, wanted %q",
			held.Engine, wanted.Engine)
	}
	if held.DefaultPort != wanted.DefaultPort {
		t.Errorf("an unknown engine answered port %d, wanted %d",
			held.DefaultPort, wanted.DefaultPort)
	}
}

// An engine that connects to a server without a user must still be able to send a
// password, or a hosted deployment cannot be opened. MongoDB is the only engine where the
// user is optional and the password is not.
func TestAnEngineWithAnOptionalUserCanStillGiveAPassword(t *testing.T) {
	held := ResolveEngineInfo(EngineMongo)
	if held.NeedsUser {
		t.Error("mongodb requires a user, which a server with authentication off refuses")
	}
	if !held.NeedsPassword {
		t.Error("mongodb gives no password, so a hosted deployment cannot be opened")
	}
}

// MongoDB supports a transaction on a replica set and on a sharded cluster, and not on a
// standalone server. The registry entry gives the capability of the engine, and the session
// reports what the connected deployment supports.
func TestMongodbNamesTheTransactionItsDeploymentsHold(t *testing.T) {
	if !ResolveEngineInfo(EngineMongo).Capabilities.HasTransactions {
		t.Error("mongodb reports that no deployment of it holds a transaction")
	}
}
