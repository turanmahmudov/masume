package core

// FindAllowed returns the matching allowed value, or false for an unknown value.
func FindAllowed[T ~string](allowed []T, written string) (T, bool) {
	for _, candidate := range allowed {
		if string(candidate) == written {
			return candidate, true
		}
	}
	var none T
	return none, false
}
