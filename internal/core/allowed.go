package core

// FindAllowed returns the entry of allowed that equals written, and false if there is
// none. Every setting that accepts one word from a fixed set uses this, so an unknown
// word gives the same result everywhere.
func FindAllowed[T ~string](allowed []T, written string) (T, bool) {
	for _, candidate := range allowed {
		if string(candidate) == written {
			return candidate, true
		}
	}
	var none T
	return none, false
}
