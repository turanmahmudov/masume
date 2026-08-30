package db

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSideConnectionOpensOnceUnderManyReaders(t *testing.T) {
	opens := atomic.Int32{}
	side := NewSideConnection(func() (int, error) {
		opens.Add(1)
		time.Sleep(time.Millisecond)
		return 7, nil
	})

	group := sync.WaitGroup{}
	for range 16 {
		group.Go(func() {
			held, err := side.Read()
			if err != nil || held != 7 {
				t.Errorf("the second connection read as %d, %v", held, err)
			}
		})
	}
	group.Wait()

	if opens.Load() != 1 {
		t.Errorf("the second connection was opened %d times, want 1", opens.Load())
	}
}
