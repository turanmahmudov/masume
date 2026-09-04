package core_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
)

// A rate is read beside other numbers on one line, so it is written short and its mark
// stands against the number with no space between them.
func TestFormatByteRateWritesTheLargestSizeThatLeavesAWholeNumber(t *testing.T) {
	for _, one := range []struct {
		perSecond float64
		written   string
	}{
		{0, "0B/s"},
		{19, "19B/s"},
		{1023, "1023B/s"},
		{1024, "1kB/s"},
		{6963, "6.8kB/s"},
		{44040192, "42MB/s"},
		{1 << 30, "1GB/s"},
		{1.5 * (1 << 40), "1.5TB/s"},
		// A rate is never below zero, whatever the counters did.
		{-5, "0B/s"},
	} {
		if held := core.FormatByteRate(one.perSecond); held != one.written {
			t.Errorf("%v a second reads %q, wanted %q", one.perSecond, held, one.written)
		}
	}
}

// The count of a rate is written the way a reader reads it, so a thousand is a k and a
// number under ten keeps the decimal that tells it from zero.
func TestFormatRateWritesACountAReaderSays(t *testing.T) {
	for _, one := range []struct {
		perSecond float64
		written   string
	}{
		{0, "0"},
		{3.2, "3.2"},
		{9.4, "9.4"},
		{10, "10"},
		{20, "20"},
		{940, "940"},
		{1200, "1.2k"},
		{12000, "12k"},
		{-1, "0"},
	} {
		if held := core.FormatRate(one.perSecond); held != one.written {
			t.Errorf("%v a second reads %q, wanted %q", one.perSecond, held, one.written)
		}
	}
}

// A share is written as a percentage of one decimal, and a share outside its own range is
// held at its ends rather than written as a number no share can be.
func TestFormatShareWritesAPercentage(t *testing.T) {
	for _, one := range []struct {
		share   float64
		written string
	}{
		{0, "0%"},
		{0.996, "99.6%"},
		{1, "100%"},
		{1.2, "100%"},
		{-0.5, "0%"},
	} {
		if held := core.FormatShare(one.share); held != one.written {
			t.Errorf("a share of %v reads %q, wanted %q", one.share, held, one.written)
		}
	}
}
