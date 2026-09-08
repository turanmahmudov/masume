package sqlserver

import (
	"fmt"
	"strconv"
	"strings"
)

// RoutineKind is a kind of routine the server stores and drops separately.
type RoutineKind string

// The two kinds the server keeps apart.
const (
	RoutineFunction  RoutineKind = "function"
	RoutineProcedure RoutineKind = "procedure"
)

// BuildRoutineIdentity combines the routine kind and name for DDL operations.
func BuildRoutineIdentity(kind RoutineKind, schema, name string) string {
	return fmt.Sprintf("%s:%s.%s", kind, schema, name)
}

// ReadRoutineKind reads the kind back out of an identity.
func ReadRoutineKind(identity string) RoutineKind {
	if strings.HasPrefix(identity, string(RoutineProcedure)+":") {
		return RoutineProcedure
	}
	return RoutineFunction
}

// BuildKillStatement writes the statement that ends another session.
func BuildKillStatement(pid int64) string {
	return "kill " + strconv.FormatInt(pid, 10)
}
