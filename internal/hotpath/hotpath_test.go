package hotpath_test

import (
	"testing"

	"github.com/lasttoss/valkey-scale-lab/internal/hotpath"
)

// TestPerMatchGivesEveryCallerItsOwnClient documents the bug as a property rather than as a story.
// A test that fails when someone reintroduces it is worth more than a paragraph that says not to.
func TestPerMatchGivesEveryCallerItsOwnClient(t *testing.T) {
	a := hotpath.PerMatch("127.0.0.1:6379", 0)
	b := hotpath.PerMatch("127.0.0.1:6379", 0)
	defer a.Close()
	defer b.Close()
	if a == b {
		t.Fatal("two calls returned one client; the leak this repository reproduces is gone, so the lab measures nothing")
	}
}

func TestSharedGivesOneClientPerAddressAndDatabase(t *testing.T) {
	s := hotpath.NewShared(hotpath.DefaultPool())
	defer s.Close()

	first := s.Client("127.0.0.1:6379", 0)
	if first != s.Client("127.0.0.1:6379", 0) {
		t.Error("the same address and database returned two clients: the singleton is not a singleton")
	}
	if s.Open() != 1 {
		t.Errorf("Open() = %d, want 1", s.Open())
	}

	// A second database is a second keyspace: reusing the first client would write to the wrong one.
	if s.Client("127.0.0.1:6379", 1) == first {
		t.Error("a different database reused the same client: the cache key is missing the database")
	}
	if s.Open() != 2 {
		t.Errorf("Open() = %d, want 2", s.Open())
	}

	// A second address is a second server, and a cache keyed by database alone would collapse them.
	if s.Client("127.0.0.1:6380", 0) == first {
		t.Error("a different address reused the same client")
	}
	if s.Open() != 3 {
		t.Errorf("Open() = %d, want 3", s.Open())
	}
}

func TestCloseReleasesEveryClient(t *testing.T) {
	s := hotpath.NewShared(hotpath.DefaultPool())
	s.Client("127.0.0.1:6379", 0)
	s.Client("127.0.0.1:6379", 1)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if s.Open() != 0 {
		t.Errorf("Open() = %d after Close, want 0", s.Open())
	}
	if err := s.Close(); err != nil {
		t.Fatalf("a second Close is not an error: %v", err)
	}
}
