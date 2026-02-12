package tui

import "testing"

func TestSetAndGetMark(t *testing.T) {
	mm := NewMarkManager()

	mm.SetMark('a', "list", 5)
	mk, ok := mm.GetMark('a')
	if !ok {
		t.Fatal("expected mark 'a' to exist")
	}
	if mk.View != "list" || mk.Cursor != 5 {
		t.Fatalf("expected list/5, got %s/%d", mk.View, mk.Cursor)
	}

	// Overwrite
	mm.SetMark('a', "detail", 0)
	mk, ok = mm.GetMark('a')
	if !ok {
		t.Fatal("expected mark 'a' to exist after overwrite")
	}
	if mk.View != "detail" || mk.Cursor != 0 {
		t.Fatalf("expected detail/0, got %s/%d", mk.View, mk.Cursor)
	}

	// Non-existent mark
	_, ok = mm.GetMark('z')
	if ok {
		t.Fatal("expected mark 'z' to not exist")
	}
}

func TestSetMarkIgnoresInvalid(t *testing.T) {
	mm := NewMarkManager()
	mm.SetMark('A', "list", 0) // uppercase — should be ignored
	mm.SetMark('1', "list", 0) // digit — should be ignored
	if len(mm.AllMarks()) != 0 {
		t.Fatal("expected no marks for invalid rune inputs")
	}
}

func TestAllMarks(t *testing.T) {
	mm := NewMarkManager()
	mm.SetMark('a', "list", 1)
	mm.SetMark('b', "sessions", 3)

	all := mm.AllMarks()
	if len(all) != 2 {
		t.Fatalf("expected 2 marks, got %d", len(all))
	}
	if all['a'].View != "list" {
		t.Fatal("mark 'a' wrong view")
	}
	if all['b'].View != "sessions" {
		t.Fatal("mark 'b' wrong view")
	}

	// Mutating the returned map should not affect the manager.
	delete(all, 'a')
	if _, ok := mm.GetMark('a'); !ok {
		t.Fatal("deleting from AllMarks() should not affect manager")
	}
}

func TestJumpListPushAndBack(t *testing.T) {
	mm := NewMarkManager()

	// Jump back with empty list returns false.
	_, ok := mm.JumpBack()
	if ok {
		t.Fatal("JumpBack on empty list should return false")
	}

	mm.PushJump("list", 0)
	mm.PushJump("list", 5)
	mm.PushJump("detail", 0)

	// Jump back from position 2 -> position 1
	mk, ok := mm.JumpBack()
	if !ok {
		t.Fatal("JumpBack should succeed")
	}
	if mk.View != "list" || mk.Cursor != 5 {
		t.Fatalf("expected list/5, got %s/%d", mk.View, mk.Cursor)
	}

	// Jump back again -> position 0
	mk, ok = mm.JumpBack()
	if !ok {
		t.Fatal("JumpBack should succeed")
	}
	if mk.View != "list" || mk.Cursor != 0 {
		t.Fatalf("expected list/0, got %s/%d", mk.View, mk.Cursor)
	}

	// Jump back again — already at oldest
	_, ok = mm.JumpBack()
	if ok {
		t.Fatal("JumpBack at oldest should return false")
	}
}

func TestJumpListForward(t *testing.T) {
	mm := NewMarkManager()

	mm.PushJump("list", 0)
	mm.PushJump("list", 5)
	mm.PushJump("detail", 0)

	// Jump back twice
	mm.JumpBack()
	mm.JumpBack()

	// Jump forward -> position 1
	mk, ok := mm.JumpForward()
	if !ok {
		t.Fatal("JumpForward should succeed")
	}
	if mk.View != "list" || mk.Cursor != 5 {
		t.Fatalf("expected list/5, got %s/%d", mk.View, mk.Cursor)
	}

	// Jump forward again -> position 2
	mk, ok = mm.JumpForward()
	if !ok {
		t.Fatal("JumpForward should succeed")
	}
	if mk.View != "detail" || mk.Cursor != 0 {
		t.Fatalf("expected detail/0, got %s/%d", mk.View, mk.Cursor)
	}

	// Jump forward at head — should return false
	_, ok = mm.JumpForward()
	if ok {
		t.Fatal("JumpForward at head should return false")
	}
}

func TestJumpListForwardTruncatesOnPush(t *testing.T) {
	mm := NewMarkManager()

	mm.PushJump("list", 0)
	mm.PushJump("list", 5)
	mm.PushJump("list", 10)

	// Jump back twice to position 0
	mm.JumpBack()
	mm.JumpBack()

	// Push a new jump — this should truncate entries after position 0
	mm.PushJump("sessions", 2)

	// Forward should fail because the old forward history was discarded
	_, ok := mm.JumpForward()
	if ok {
		t.Fatal("JumpForward should fail after push truncated forward history")
	}

	// Jump back should go to list/0
	mk, ok := mm.JumpBack()
	if !ok {
		t.Fatal("JumpBack should succeed")
	}
	if mk.View != "list" || mk.Cursor != 0 {
		t.Fatalf("expected list/0, got %s/%d", mk.View, mk.Cursor)
	}
}

func TestJumpListOverflow(t *testing.T) {
	mm := NewMarkManager()

	// Push 150 entries — only the last 100 should remain
	for i := 0; i < 150; i++ {
		mm.PushJump("list", i)
	}

	if len(mm.jumpList) != jumpListMaxSize {
		t.Fatalf("expected jump list size %d, got %d", jumpListMaxSize, len(mm.jumpList))
	}

	// The oldest entry should be index 50 (entries 0-49 were trimmed)
	oldest := mm.jumpList[0]
	if oldest.Cursor != 50 {
		t.Fatalf("expected oldest cursor 50, got %d", oldest.Cursor)
	}

	// The newest entry should be index 149
	newest := mm.jumpList[len(mm.jumpList)-1]
	if newest.Cursor != 149 {
		t.Fatalf("expected newest cursor 149, got %d", newest.Cursor)
	}
}

func TestLastJump(t *testing.T) {
	mm := NewMarkManager()

	// Empty list
	_, ok := mm.LastJump()
	if ok {
		t.Fatal("LastJump on empty list should return false")
	}

	// Single entry — not enough for "last"
	mm.PushJump("list", 0)
	_, ok = mm.LastJump()
	if ok {
		t.Fatal("LastJump with single entry should return false")
	}

	// Two entries — last jump should return the first entry
	mm.PushJump("list", 10)
	mk, ok := mm.LastJump()
	if !ok {
		t.Fatal("LastJump should succeed with 2 entries")
	}
	if mk.View != "list" || mk.Cursor != 0 {
		t.Fatalf("expected list/0, got %s/%d", mk.View, mk.Cursor)
	}
}

func TestLastJumpAfterJumpBack(t *testing.T) {
	mm := NewMarkManager()

	mm.PushJump("list", 0)
	mm.PushJump("list", 5)
	mm.PushJump("detail", 10)

	// Jump back once: index goes from 2 to 1
	mm.JumpBack()

	// LastJump should return the entry before index 1, which is index 0
	mk, ok := mm.LastJump()
	if !ok {
		t.Fatal("LastJump should succeed")
	}
	if mk.View != "list" || mk.Cursor != 0 {
		t.Fatalf("expected list/0, got %s/%d", mk.View, mk.Cursor)
	}
}
