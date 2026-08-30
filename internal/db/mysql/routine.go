package mysql

import (
	"fmt"
	"strings"
)

// RoutineKind is a kind of routine MySQL stores and drops separately.
type RoutineKind string

// The two kinds MySQL keeps apart.
const (
	RoutineFunction  RoutineKind = "function"
	RoutineProcedure RoutineKind = "procedure"
)

// BuildRoutineIdentity names a routine, because the name alone does not give the kind
// and MySQL needs it for the DDL and the DROP.
func BuildRoutineIdentity(kind RoutineKind, schema, name string) string {
	return fmt.Sprintf("%s:%s.%s", kind, schema, name)
}

// ReadRoutineKind reads the kind back out of an identity.
func ReadRoutineKind(identity string) RoutineKind {
	if strings.HasPrefix(identity, "procedure:") {
		return RoutineProcedure
	}
	return RoutineFunction
}
