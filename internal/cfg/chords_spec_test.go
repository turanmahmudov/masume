package cfg_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

// A chord in the config file must parse into the key press the user makes, or the key is
// not bound and the action cannot be started.
func TestParseChordReadsWhatTheConfigFileWrites(t *testing.T) {
	for _, held := range []struct {
		text  string
		key   string
		ctrl  bool
		meta  bool
		shift bool
	}{
		{"a", "a", false, false, false},
		{"ctrl+a", "a", true, false, false},
		{"alt+a", "a", false, true, false},
		{"shift+a", "a", false, false, true},
		{"ctrl+shift+a", "a", true, false, true},
		{"ctrl+alt+shift+a", "a", true, true, true},

		// A named key parses into the name the terminal reports, so Enter and Return
		// give the same chord and both spellings run the same action.
		{"enter", "return", false, false, false},
		{"return", "return", false, false, false},
		// A modifier is parsed in upper case and in lower case.
		{"CTRL+a", "a", true, false, false},
		// A single capital letter is the config file form of shift.
		{"ctrl+A", "a", true, false, true},
		{"f5", "f5", false, false, false},
		{"ctrl+f12", "f12", true, false, false},

		// The order of the modifiers does not matter.
		{"shift+ctrl+a", "a", true, false, true},
	} {
		t.Run(held.text, func(t *testing.T) {
			chord, is := cfg.ParseChord(held.text)
			if !is {
				t.Fatalf("%q was not read as a chord", held.text)
			}
			if chord.Key != held.key {
				t.Errorf("the key reads %q, wanted %q", chord.Key, held.key)
			}
			if chord.Ctrl != held.ctrl || chord.Meta != held.meta || chord.Shift != held.shift {
				t.Errorf("the modifiers read ctrl=%v alt=%v shift=%v, wanted %v %v %v",
					chord.Ctrl, chord.Meta, chord.Shift, held.ctrl, held.meta, held.shift)
			}
		})
	}
}

func TestParseChordRefusesWhatIsNotAChord(t *testing.T) {
	for _, text := range []string{
		"",
		"   ",
		"+",        // a plus sign alone is no key
		"ctrl+",    // a modifier and no key
		"ctrl",     // a modifier and no key
		"hyper+a",  // a modifier no terminal sends
		"ctrl+a+b", // two keys in one chord
	} {
		if _, is := cfg.ParseChord(text); is {
			t.Errorf("%q was read as a chord", text)
		}
	}
}

// An empty part between two plus signs is skipped, so a binding with an extra plus sign
// still runs its action and does not stay unbound.
func TestParseChordDropsAnEmptyPartBetweenPluses(t *testing.T) {
	plain, is := cfg.ParseChord("a")
	if !is {
		t.Fatal("a plain key was not read")
	}
	for _, text := range []string{"+a", "a+"} {
		held, is := cfg.ParseChord(text)
		if !is {
			t.Errorf("%q was not read", text)
			continue
		}
		if held != plain {
			t.Errorf("%q read as %+v, wanted the same chord as %q", text, held, "a")
		}
	}

	withCtrl, is := cfg.ParseChord("ctrl+a")
	if !is {
		t.Fatal("ctrl+a was not read")
	}
	if held, is := cfg.ParseChord("ctrl++a"); !is || held != withCtrl {
		t.Errorf("ctrl++a read as %+v, wanted the same chord as ctrl+a", held)
	}
}

// A chord converts back into the text it was parsed from, so the help and the hints show
// the key the user has to press.
func TestDescribeChordRoundTrips(t *testing.T) {
	for _, text := range []string{"a", "ctrl+a", "alt+a", "shift+a", "ctrl+shift+a", "return", "f5"} {
		chord, is := cfg.ParseChord(text)
		if !is {
			t.Fatalf("%q was not read as a chord", text)
		}
		written := cfg.DescribeChord(chord)
		again, is := cfg.ParseChord(written)
		if !is {
			t.Fatalf("%q was described as %q, which does not read back", text, written)
		}
		if again != chord {
			t.Errorf("%q described as %q read back as another chord", text, written)
		}
	}
}

// A sequence is two key presses in a row, for example after a leader key.
func TestParseChordSequenceReadsSeveralPressesInOrder(t *testing.T) {
	sequence, is := cfg.ParseChordSequence("ctrl+k b")
	if !is {
		t.Fatal("a sequence of two presses was not read")
	}
	if len(sequence) != 2 {
		t.Fatalf("the sequence holds %d presses, wanted 2", len(sequence))
	}
	if !sequence[0].Ctrl || sequence[0].Key != "k" {
		t.Errorf("the first press reads %+v", sequence[0])
	}
	if sequence[1].Key != "b" {
		t.Errorf("the second press reads %+v", sequence[1])
	}
}

func TestParseChordSequenceRefusesASequenceWithABadPress(t *testing.T) {
	for _, text := range []string{"", "   ", "ctrl+k hyper+b", "ctrl+k +"} {
		if _, is := cfg.ParseChordSequence(text); is {
			t.Errorf("%q was read as a sequence", text)
		}
	}
}

func TestDescribeSequenceNamesEveryPress(t *testing.T) {
	sequence, is := cfg.ParseChordSequence("ctrl+k b")
	if !is {
		t.Fatal("the sequence was not read")
	}
	written := cfg.DescribeSequence(sequence)
	again, is := cfg.ParseChordSequence(written)
	if !is {
		t.Fatalf("the sequence described as %q does not read back", written)
	}
	if len(again) != len(sequence) {
		t.Errorf("%q read back as %d presses, wanted %d", written, len(again), len(sequence))
	}
}

// A chord no terminal sends cannot be bound, because the action would never run and the
// user would think the client ignores the key.
func TestFindUndeliverableChordNamesWhatATerminalWillNotReport(t *testing.T) {
	// A plain letter and a control chord both arrive, so both are allowed.
	for _, text := range []string{"a", "ctrl+a", "enter", "f5"} {
		chord, is := cfg.ParseChord(text)
		if !is {
			t.Fatalf("%q was not read", text)
		}
		if reason := cfg.FindUndeliverableChord(chord); reason != "" {
			t.Errorf("%q was refused: %s", text, reason)
		}
	}
}

// The key of an action contains its scope and its name and splits back into the two, so the
// config file can address one action of one pane.
func TestBuildAndSplitActionKeyRoundTrip(t *testing.T) {
	for _, held := range []struct {
		scope cfg.KeyScope
		id    string
	}{
		{cfg.ScopeGlobal, "run-statement"},
		{cfg.ScopeGrid, "copy-cell"},
		{cfg.ScopeTree, "open-relation"},
	} {
		key := cfg.BuildActionKey(held.scope, held.id)
		scope, id := cfg.SplitActionKey(key)
		if scope != held.scope || id != held.id {
			t.Errorf("%q split into %q and %q, wanted %q and %q",
				key, scope, id, held.scope, held.id)
		}
	}
}

// A URL pasted into the connection form fills the fields, so the user does not type a value
// the URL already contains.
func TestParseConnectionUrlReadsEveryPart(t *testing.T) {
	held, is := cfg.ParseConnectionURL("postgres://reader@db.example.com:6543/shop")
	if !is {
		t.Fatal("a postgres URL was not read")
	}
	if held.Engine != core.EnginePostgres {
		t.Errorf("the engine reads %q", held.Engine)
	}
	if held.Host != "db.example.com" {
		t.Errorf("the host reads %q", held.Host)
	}
	if held.Port != 6543 {
		t.Errorf("the port reads %d", held.Port)
	}
	if held.Database != "shop" {
		t.Errorf("the database reads %q", held.Database)
	}
	if held.User != "reader" {
		t.Errorf("the user reads %q", held.User)
	}
}

// A URL without a port uses the default port of the engine.
func TestParseConnectionUrlFallsBackToTheDefaultPort(t *testing.T) {
	held, is := cfg.ParseConnectionURL("postgres://reader@localhost/shop")
	if !is {
		t.Fatal("the URL was not read")
	}
	if held.Port != core.ResolveDefaultPort(core.EnginePostgres) {
		t.Errorf("the port reads %d, wanted the default of the engine", held.Port)
	}
}

// A URL puts an IPv6 address in brackets. Every other reader needs it without them.
func TestParseConnectionUrlTakesTheBracketsOffAnIpv6Address(t *testing.T) {
	held, is := cfg.ParseConnectionURL("postgres://reader@[::1]:5432/shop")
	if !is {
		t.Fatal("an IPv6 URL was not read")
	}
	if held.Host != "::1" {
		t.Errorf("the host reads %q, wanted it without brackets", held.Host)
	}
}

// Every scheme of an engine must be parsed, or a URL printed by a server cannot be pasted.
func TestParseConnectionUrlReadsEverySchemeAnEngineClaims(t *testing.T) {
	for _, info := range core.ListEngineInfo() {
		for _, scheme := range info.URLSchemes {
			written := scheme + "://reader@localhost/shop"
			held, is := cfg.ParseConnectionURL(written)
			if !is {
				t.Errorf("%q was not read", written)
				continue
			}
			if held.Engine != info.Engine {
				t.Errorf("%q reads as %q, wanted %q", written, held.Engine, info.Engine)
			}
		}
	}
}

func TestParseConnectionUrlRefusesWhatItCannotUse(t *testing.T) {
	for _, text := range []string{
		"",
		"not a url",
		"db.example.com/shop",                   // no scheme
		"oracle://reader@localhost/shop",        // a scheme of no engine
		"postgres://reader@localhost",           // no database
		"postgres://reader@/shop",               // no host
		"postgres://reader@localhost:0/shop",    // a port below one
		"postgres://reader@localhost:nope/shop", // a port that is not a number",
	} {
		if _, is := cfg.ParseConnectionURL(text); is {
			t.Errorf("%q was read as a connection URL", text)
		}
	}
}
