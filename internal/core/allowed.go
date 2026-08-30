package core

// FindAllowed returns the value of the list this text names, and nothing where the list
// holds none. Every setting that reads a word out of a fixed set is read through this, so
// a name the list does not hold is answered the same way everywhere.
func FindAllowed[T ~string](allowed []T, written string) (T, bool) {
	for _, candidate := range allowed {
		if string(candidate) == written {
			return candidate, true
		}
	}
	var none T
	return none, false
}
