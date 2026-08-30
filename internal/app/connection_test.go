package app

import (
	"testing"
	"time"
)

func TestDropStaleNoticeKeepsAnErrorTwiceAsLong(t *testing.T) {
	shownAt := time.Now()
	cases := []struct {
		name  string
		tone  NoticeTone
		after time.Duration
		gone  bool
	}{
		{"an info under its life stays", NoticeInfo, 3 * time.Second, false},
		{"an info over its life goes", NoticeInfo, 5 * time.Second, true},
		{"an error under its life stays", NoticeError, 5 * time.Second, false},
		{"an error at its life stays", NoticeError, 8 * time.Second, false},
		{"an error over its life goes", NoticeError, 9 * time.Second, true},
	}

	for _, held := range cases {
		t.Run(held.name, func(t *testing.T) {
			connection := &Connection{
				Notice: &Notice{Text: "a report", Tone: held.tone, ShownAt: shownAt},
			}
			connection.DropStaleNotice(shownAt.Add(held.after))
			if gone := connection.Notice == nil; gone != held.gone {
				t.Errorf("after %s the report gone is %v, want %v",
					held.after, gone, held.gone)
			}
		})
	}
}
