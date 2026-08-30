package ui

import (
	"testing"
	"time"
)

func TestResolveHealthBackoff(t *testing.T) {
	interval := 30 * time.Second
	cases := []struct {
		failures int
		wanted   time.Duration
	}{
		{0, 30 * time.Second},
		{1, 30 * time.Second},
		{2, 60 * time.Second},
		{3, 60 * time.Second},
		{9, 60 * time.Second},
	}
	for _, held := range cases {
		if answered := ResolveHealthBackoff(held.failures, interval); answered != held.wanted {
			t.Errorf("%d failures gave %s, wanted %s", held.failures, answered, held.wanted)
		}
	}
	if answered := ResolveHealthBackoff(1, 0); answered != time.Second {
		t.Errorf("no interval gave %s, wanted 1s", answered)
	}
}
