package session

import (
	"bytes"
	"sync"
	"testing"
)

// --------------------------------------------------------------------------
// ringBuffer tests
// --------------------------------------------------------------------------

func TestRingBuffer_WriteAndDrain(t *testing.T) {
	rb := newRingBuffer(4)

	rb.Write([]byte("hello"))
	rb.Write([]byte("world"))

	got := rb.Drain()
	want := []byte("helloworld")
	if !bytes.Equal(got, want) {
		t.Errorf("Drain: want %q, got %q", want, got)
	}
}

func TestRingBuffer_Drain_EmptyReturnsNil(t *testing.T) {
	rb := newRingBuffer(4)
	if got := rb.Drain(); got != nil {
		t.Errorf("Drain on empty buffer: want nil, got %v", got)
	}
}

func TestRingBuffer_Write_EmptySlice_NoOp(t *testing.T) {
	rb := newRingBuffer(4)
	rb.Write([]byte{})
	if got := rb.Drain(); got != nil {
		t.Errorf("Write empty: expected nil drain, got %v", got)
	}
}

func TestRingBuffer_OverwritesOldestWhenFull(t *testing.T) {
	// Capacity 3: write 4 items — oldest (item 0) is overwritten.
	rb := newRingBuffer(3)
	rb.Write([]byte("A"))
	rb.Write([]byte("B"))
	rb.Write([]byte("C"))
	rb.Write([]byte("D")) // overwrites "A"

	got := rb.Drain()
	want := []byte("BCD")
	if !bytes.Equal(got, want) {
		t.Errorf("overwrite oldest: want %q, got %q", want, got)
	}
}

func TestRingBuffer_Clear_EmptiesBuffer(t *testing.T) {
	rb := newRingBuffer(4)
	rb.Write([]byte("data"))
	rb.Clear()
	if got := rb.Drain(); got != nil {
		t.Errorf("after Clear, Drain should return nil, got %v", got)
	}
}

func TestRingBuffer_Clear_EmptyBuffer_NoOp(t *testing.T) {
	rb := newRingBuffer(4)
	// Should not panic on empty buffer
	rb.Clear()
}

func TestRingBuffer_DrainTwice_SecondIsNil(t *testing.T) {
	rb := newRingBuffer(4)
	rb.Write([]byte("once"))
	rb.Drain()
	if got := rb.Drain(); got != nil {
		t.Errorf("second Drain should return nil, got %v", got)
	}
}

func TestRingBuffer_Concurrent_WriteAndDrain(t *testing.T) {
	rb := newRingBuffer(64)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rb.Write([]byte("x"))
		}()
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rb.Drain()
		}()
	}
	wg.Wait() // must not race or panic
}

// --------------------------------------------------------------------------
// replayBuffer tests
// --------------------------------------------------------------------------

func TestReplayBuffer_WriteAndSnapshot(t *testing.T) {
	rb := newReplayBuffer(1024)
	rb.Write([]byte("hello "))
	rb.Write([]byte("world"))

	snap := rb.Snapshot()
	want := []byte("hello world")
	if !bytes.Equal(snap, want) {
		t.Errorf("Snapshot: want %q, got %q", want, snap)
	}
}

func TestReplayBuffer_Snapshot_EmptyReturnsNil(t *testing.T) {
	rb := newReplayBuffer(1024)
	if got := rb.Snapshot(); got != nil {
		t.Errorf("Snapshot on empty: want nil, got %v", got)
	}
}

func TestReplayBuffer_Write_EmptySlice_NoOp(t *testing.T) {
	rb := newReplayBuffer(1024)
	rb.Write([]byte{})
	if got := rb.Snapshot(); got != nil {
		t.Errorf("Write empty: expected nil snapshot, got %v", got)
	}
}

func TestReplayBuffer_SnapshotDoesNotClear(t *testing.T) {
	rb := newReplayBuffer(1024)
	rb.Write([]byte("keep"))
	rb.Snapshot()
	if got := rb.Snapshot(); !bytes.Equal(got, []byte("keep")) {
		t.Errorf("second Snapshot should return same data, got %q", got)
	}
}

func TestReplayBuffer_SnapshotReturnsCopy(t *testing.T) {
	rb := newReplayBuffer(1024)
	rb.Write([]byte("original"))
	snap := rb.Snapshot()
	// Mutate the snapshot — should not affect the buffer
	snap[0] = 'X'
	snap2 := rb.Snapshot()
	if snap2[0] != 'o' {
		t.Errorf("snapshot mutation leaked into buffer: got %q", snap2[0])
	}
}

func TestReplayBuffer_TruncatesAtMaxCapacity(t *testing.T) {
	// max of 5 bytes
	rb := newReplayBuffer(5)
	rb.Write([]byte("ABCDE")) // exactly at limit
	rb.Write([]byte("FGH"))   // 3 more → 8 total → truncates to last 5: "DEFGH"

	snap := rb.Snapshot()
	if len(snap) > 5 {
		t.Errorf("replayBuffer exceeded max capacity: len=%d, want <=5", len(snap))
	}
	// The last 5 bytes should be present
	want := []byte("DEFGH")
	if !bytes.Equal(snap, want) {
		t.Errorf("truncated snapshot: want %q, got %q", want, snap)
	}
}

func TestReplayBuffer_Concurrent_WriteAndSnapshot(t *testing.T) {
	rb := newReplayBuffer(4096)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rb.Write([]byte("data"))
		}()
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = rb.Snapshot()
		}()
	}
	wg.Wait() // must not race or panic
}

// --------------------------------------------------------------------------
// SessionStatus.String tests
// --------------------------------------------------------------------------

func TestSessionStatus_String(t *testing.T) {
	cases := []struct {
		status SessionStatus
		want   string
	}{
		{StatusConnecting, "connecting"},
		{StatusConnected, "connected"},
		{StatusDetached, "detached"},
		{StatusClosed, "closed"},
		{SessionStatus(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.status.String(); got != tc.want {
			t.Errorf("SessionStatus(%d).String(): want %q, got %q", tc.status, tc.want, got)
		}
	}
}
