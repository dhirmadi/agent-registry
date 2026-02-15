package mcp

import (
	"sync"
	"testing"
)

func TestSessionStoreCreateAndGet(t *testing.T) {
	store := NewSessionStore()

	session, err := store.NewSession(nil)
	if err != nil {
		t.Fatalf("NewSession error: %v", err)
	}
	if session.ID == "" {
		t.Fatal("session ID should not be empty")
	}
	if len(session.ID) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("session ID length: got %d, want 64", len(session.ID))
	}

	got, ok := store.GetSession(session.ID)
	if !ok {
		t.Fatal("session should exist")
	}
	if got.ID != session.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, session.ID)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestSessionStoreGetNonexistent(t *testing.T) {
	store := NewSessionStore()

	_, ok := store.GetSession("nonexistent-id")
	if ok {
		t.Error("should not find nonexistent session")
	}
}

func TestSessionStoreDelete(t *testing.T) {
	store := NewSessionStore()

	session, _ := store.NewSession(nil)
	store.DeleteSession(session.ID)

	_, ok := store.GetSession(session.ID)
	if ok {
		t.Error("session should be deleted")
	}
}

func TestSessionStoreDeleteNonexistent(t *testing.T) {
	store := NewSessionStore()

	// Should not panic
	store.DeleteSession("nonexistent-id")
}

func TestSessionStoreUniqueIDs(t *testing.T) {
	store := NewSessionStore()
	ids := make(map[string]bool)

	for i := 0; i < 100; i++ {
		session, err := store.NewSession(nil)
		if err != nil {
			t.Fatalf("NewSession error on iteration %d: %v", i, err)
		}
		if ids[session.ID] {
			t.Fatalf("duplicate session ID on iteration %d: %s", i, session.ID)
		}
		ids[session.ID] = true
	}
}

func TestSessionStoreClientCapabilities(t *testing.T) {
	store := NewSessionStore()

	caps := &ClientCapabilities{
		Roots: &RootsCapability{ListChanged: true},
	}
	session, err := store.NewSession(caps)
	if err != nil {
		t.Fatalf("NewSession error: %v", err)
	}

	got, ok := store.GetSession(session.ID)
	if !ok {
		t.Fatal("session should exist")
	}
	if got.ClientCapabilities == nil {
		t.Fatal("client capabilities should be set")
	}
	if got.ClientCapabilities.Roots == nil {
		t.Fatal("roots capability should be set")
	}
	if !got.ClientCapabilities.Roots.ListChanged {
		t.Error("roots.listChanged should be true")
	}
}

func TestSessionStoreConcurrentAccess(t *testing.T) {
	store := NewSessionStore()
	var wg sync.WaitGroup

	// Create sessions concurrently
	sessions := make([]string, 50)
	var mu sync.Mutex

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			session, err := store.NewSession(nil)
			if err != nil {
				t.Errorf("NewSession error: %v", err)
				return
			}
			mu.Lock()
			sessions[idx] = session.ID
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	// Verify all sessions exist
	for i, id := range sessions {
		if id == "" {
			t.Errorf("session %d has empty ID", i)
			continue
		}
		_, ok := store.GetSession(id)
		if !ok {
			t.Errorf("session %d not found: %s", i, id)
		}
	}

	// Delete half concurrently
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			store.DeleteSession(sessions[idx])
		}(i)
	}
	wg.Wait()

	// Verify deletions
	for i := 0; i < 25; i++ {
		_, ok := store.GetSession(sessions[i])
		if ok {
			t.Errorf("session %d should be deleted", i)
		}
	}
	for i := 25; i < 50; i++ {
		_, ok := store.GetSession(sessions[i])
		if !ok {
			t.Errorf("session %d should still exist", i)
		}
	}
}

func TestSessionIDFormat(t *testing.T) {
	store := NewSessionStore()
	session, _ := store.NewSession(nil)

	// Verify hex format: only contains [0-9a-f]
	for _, c := range session.ID {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("session ID contains non-hex character: %c", c)
			break
		}
	}
}
