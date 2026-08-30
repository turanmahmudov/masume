package engines

import (
	"context"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"

	"github.com/turanmahmudov/masume/internal/core"
)

// Every engine the client offers needs an adapter and an entry, or picking it in the palette
// answers "no driver opens this engine". This walks the whole list, so an engine added to
// core.Engines and forgotten here fails at once.
func TestEveryEngineHasAnAdapterAndSupport(t *testing.T) {
	adapters := CreateAdapters()
	for _, engine := range core.Engines {
		if _, known := adapters[engine]; !known {
			t.Errorf("%q has no adapter", engine)
		}
		support := ResolveSupport(engine)
		if support.Engine != engine {
			t.Errorf("%q answers support for %q", engine, support.Engine)
			continue
		}
		if support.Dialect == nil {
			t.Errorf("%q answers no dialect", engine)
		}
		if support.Language == nil {
			t.Errorf("%q answers no language", engine)
		}
	}
}

// The adapters hold no engine the client does not offer, or the palette and the registry
// would disagree about what can be opened.
func TestTheAdaptersHoldNoEngineTheClientDoesNotOffer(t *testing.T) {
	offered := map[core.Engine]bool{}
	for _, engine := range core.Engines {
		offered[engine] = true
	}
	for engine := range CreateAdapters() {
		if !offered[engine] {
			t.Errorf("%q has an adapter and is not offered", engine)
		}
	}
}

// A server that speaks one protocol takes the dialect of that protocol, which is what lets
// one adapter serve a whole family.
func TestAFamilyShareOneDialect(t *testing.T) {
	byFamily := map[core.Family][]core.Engine{}
	for _, engine := range core.Engines {
		byFamily[core.ResolveEngineInfo(engine).Family] = append(
			byFamily[core.ResolveEngineInfo(engine).Family], engine)
	}

	for family, engines := range byFamily {
		first := ResolveSupport(engines[0]).Dialect
		for _, engine := range engines[1:] {
			if held := ResolveSupport(engine).Dialect; held != first {
				t.Errorf("%q of the %q family answers a dialect of its own", engine, family)
			}
		}
	}
}

// The support of an engine carries the entry of that engine, so the capabilities and the port
// a session reports are its own and not those of the base of its family.
func TestSupportCarriesTheEntryOfItsOwnEngine(t *testing.T) {
	for _, engine := range core.Engines {
		support := ResolveSupport(engine)
		wanted := core.ResolveEngineInfo(engine)
		if support.DefaultPort != wanted.DefaultPort {
			t.Errorf("%q answers port %d, wanted %d",
				engine, support.DefaultPort, wanted.DefaultPort)
		}
		if support.Capabilities != wanted.Capabilities {
			t.Errorf("%q answers capabilities that are not its own", engine)
		}
	}
}

// An engine nothing knows cannot be opened, and the refusal names it.
func TestOpenRefusesAnEngineWithNoAdapter(t *testing.T) {
	_, err := CreateAdapters().Open(context.Background(), buildUnknownProfile(), "")
	if err == nil {
		t.Fatal("an engine with no adapter opened a session")
	}
	if !containsText(err.Error(), "oracle") {
		t.Errorf("the refusal reads %q and does not name the engine", err)
	}
}

// buildUnknownProfile answers a profile on an engine the client does not offer.
func buildUnknownProfile() cfg.Profile {
	return cfg.Profile{Name: "held", Engine: core.Engine("oracle")}
}

func containsText(written, wanted string) bool {
	return strings.Contains(written, wanted)
}
