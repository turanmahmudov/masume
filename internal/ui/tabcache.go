package ui

type tabCaches struct {
	// The rows of each tab as the grid writes them, kept between frames and keyed by tab.
	text map[tabKey]gridText
	// The head of the result of each tab, kept for the same reason and keyed the same way.
	head map[tabKey]gridHead
	// The faults the scanner found in the buffer of each tab, kept because reading them
	// tokenizes the buffer and checks the catalog, and the frame reads them for the fault
	// row, the marks over the text and the gutter.
	faults map[tabKey]editorFaults
}

func (caches *tabCaches) readText(key tabKey) (gridText, bool) {
	held, found := caches.text[key]
	return held, found
}

func (caches *tabCaches) keepText(key tabKey, held gridText) {
	caches.text = keepInTabCache(caches.text, key, held)
}

func (caches *tabCaches) readHead(key tabKey) (gridHead, bool) {
	held, found := caches.head[key]
	return held, found
}

func (caches *tabCaches) keepHead(key tabKey, held gridHead) {
	caches.head = keepInTabCache(caches.head, key, held)
}

func (caches *tabCaches) readFaults(key tabKey) (editorFaults, bool) {
	held, found := caches.faults[key]
	return held, found
}

func (caches *tabCaches) keepFaults(key tabKey, held editorFaults) {
	caches.faults = keepInTabCache(caches.faults, key, held)
}

func (caches *tabCaches) forgetClosed(open func(key tabKey) bool) {
	forgetClosedInTabCache(caches.text, open)
	forgetClosedInTabCache(caches.head, open)
	forgetClosedInTabCache(caches.faults, open)
}

func keepInTabCache[T any](held map[tabKey]T, key tabKey, value T) map[tabKey]T {
	if held == nil {
		held = map[tabKey]T{}
	}
	held[key] = value
	return held
}

func forgetClosedInTabCache[T any](held map[tabKey]T, open func(key tabKey) bool) {
	for key := range held {
		if !open(key) {
			delete(held, key)
		}
	}
}
