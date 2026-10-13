package keywalk_test

import (
	"context"
	"sort"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/lasttoss/valkey-scale-lab/internal/keywalk"
)

// TestScanFindsWhatKeysFinds is the only claim SCAN has to make: the same set. Everything else
// about SCAN is a trade-off, and a trade-off that returns a different answer is not a trade-off.
//
// What this test cannot prove, because miniredis returns every matching key in one batch with
// cursor 0: that the loop follows a cursor that is not zero. That half is checked against the real
// server by `make measure`, which asserts that a SCAN sweep over 200,600 keys comes back with all
// of them and that cmdstat_keys did not move. A fake server that never paginates cannot test
// pagination, and pretending otherwise is how a loop that reads one page ships.
func TestScanFindsWhatKeysFinds(t *testing.T) {
	mr := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer c.Close()
	ctx := context.Background()

	// Enough keys that a single SCAN batch cannot be the whole answer: with 300 keys and a count of
	// 10, a loop that stops at the first cursor returns 10 of them and looks fine on a small set.
	for i := 0; i < 300; i++ {
		if err := c.Set(ctx, "lab:key:"+itoa(i), "x", 0).Err(); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Set(ctx, "other:1", "x", 0).Err(); err != nil {
		t.Fatal(err)
	}

	keys, err := keywalk.Keys(ctx, c, "lab:*")
	if err != nil {
		t.Fatal(err)
	}
	scan, err := keywalk.Scan(ctx, c, "lab:*", 10)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(keys)
	sort.Strings(scan)
	if len(keys) != 300 {
		t.Fatalf("KEYS returned %d keys, want 300", len(keys))
	}
	if len(scan) != len(keys) {
		t.Fatalf("SCAN returned %d keys, KEYS returned %d: a walk that stops early looks correct on a small set", len(scan), len(keys))
	}
	for i := range keys {
		if keys[i] != scan[i] {
			t.Fatalf("SCAN and KEYS disagree at %d: %q != %q", i, scan[i], keys[i])
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
