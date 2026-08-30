package core

import "testing"

// Every engine the client offers needs an entry, or it cannot be opened at all. This is the
// one test that walks the whole list, so a new engine added to Engines without an entry fails
// here rather than at the moment a user picks it.
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
		// A server is reached over a port; a file is opened by path and needs none.
		if info.OpensFile {
			if info.DefaultPort != 0 {
				t.Errorf("%q opens a file and still names port %d", engine, info.DefaultPort)
			}
			continue
		}
		if info.DefaultPort <= 0 {
			t.Errorf("%q reaches a server and names no port", engine)
		}
		// A hosted engine answers no scheme of its own: it is reached by the scheme of the
		// protocol it speaks, which the base engine of its family owns.
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

// Every family that reaches a server owns a scheme, so a URL can be pasted for one of its
// engines. A family that opens a file has a path instead and needs none.
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

// A capability the engine has not must stay false, because the keys it belongs to are left
// unbound rather than reporting a refusal.
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

		// A key store keeps its own order and drives no transaction the client can see.
		{EngineRedis, "sorts a read", ResolveEngineInfo(EngineRedis).Capabilities.SortsRead, false},
		{EngineRedis, "has transactions", ResolveEngineInfo(EngineRedis).Capabilities.HasTransactions, false},
		{EngineRedis, "plans a statement", ResolveEngineInfo(EngineRedis).Capabilities.PlansStatement, false},

		// A file has no server sessions to list or cancel.
		{EngineSqlite, "has server sessions", ResolveEngineInfo(EngineSqlite).Capabilities.HasServerSessions, false},
		{EngineSqlite, "cancels a running query", ResolveEngineInfo(EngineSqlite).Capabilities.CancelsRunningQuery, false},
	} {
		if held.got != held.want {
			t.Errorf("%q %s reads %v, wanted %v", held.engine, held.field, held.got, held.want)
		}
	}
}

// Cancelling needs a second connection to the same server, so an engine that reports it must
// also be one the client can open twice.
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
		// A prefix covers the schemas a server makes for itself, such as pg_toast_temp_1.
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

// An engine nothing knows answers the entry of the default engine, so a caller never reads a
// port of zero or a family of nothing. The name of the engine is validated where a profile is
// built, so this is the floor under that and not the way a bad name is caught.
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

// An engine that reaches a server and names no user of its own still has to be able to
// give a password, or a hosted deployment could not be opened at all. MongoDB is the one
// engine where the user is optional and the password is not.
func TestAnEngineWithAnOptionalUserCanStillGiveAPassword(t *testing.T) {
	held := ResolveEngineInfo(EngineMongo)
	if held.NeedsUser {
		t.Error("mongodb requires a user, which a server with authentication off refuses")
	}
	if !held.NeedsPassword {
		t.Error("mongodb gives no password, so a hosted deployment cannot be opened")
	}
}

// A password belongs to a named user on every server but one. Redis checks a password
// that names nobody, so a profile that names no user can still hold one, and the client
// must not decide there is nothing to ask for.
func TestOnlyRedisTakesAPasswordThatNamesNobody(t *testing.T) {
	for _, info := range ListEngineInfo() {
		wanted := info.Engine == EngineRedis
		if info.PasswordWithoutUser != wanted {
			t.Errorf("%q takes a password that names nobody: %v, wanted %v",
				info.Engine, info.PasswordWithoutUser, wanted)
		}
	}
}

// MongoDB holds a transaction on a replica set and on a sharded cluster, and none on a
// standalone server. The entry names what the engine can do, and the session reports what
// the deployment it reached actually answered.
func TestMongodbNamesTheTransactionItsDeploymentsHold(t *testing.T) {
	if !ResolveEngineInfo(EngineMongo).Capabilities.HasTransactions {
		t.Error("mongodb reports that no deployment of it holds a transaction")
	}
}
