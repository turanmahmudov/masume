package mysql

import (
	"strings"
	"testing"
)

func TestRoutineIdentityRoundTrips(t *testing.T) {
	for _, kind := range []RoutineKind{RoutineFunction, RoutineProcedure} {
		identity := BuildRoutineIdentity(kind, "shop", "total_of")
		if held := ReadRoutineKind(identity); held != kind {
			t.Errorf("%q reads back as %q, wanted %q", identity, held, kind)
		}
		if !strings.Contains(identity, "total_of") {
			t.Errorf("%q does not hold the name of the routine", identity)
		}
	}
}

func TestReadRoutineKindFallsBackForWhatItCannotRead(t *testing.T) {
	for _, identity := range []string{"", "nothing", "shop.total_of"} {
		held := ReadRoutineKind(identity)
		if held != RoutineFunction && held != RoutineProcedure {
			t.Errorf("%q reads as %q, which is neither kind", identity, held)
		}
	}
}
