package limits_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lasttoss/valkey-scale-lab/internal/limits"
)

// The fixture is a real row, tabs and all, because the tabs are what a naive strings.Split on a
// single space gets wrong.
const realLimits = `Limit                     Soft Limit           Hard Limit           Units
Max cpu time              unlimited            unlimited            seconds
Max open files            1024                 1048576              files
Max locked memory         65536                65536                bytes
`

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "limits")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadParsesTheOpenFilesRow(t *testing.T) {
	got, err := limits.Read(write(t, realLimits))
	if err != nil {
		t.Fatal(err)
	}
	if got.SoftOpenFiles != 1024 || got.HardOpenFiles != 1048576 {
		t.Errorf("Read() = %+v, want soft 1024 hard 1048576", got)
	}
}

func TestShortBlamesTheLimitWhenThereIsOne(t *testing.T) {
	l := limits.Limits{SoftOpenFiles: 1024}
	if _, short := l.Short(600); short {
		t.Error("600 sockets fit under 1024; that is not a warning")
	}
	msg, short := l.Short(2000)
	if !short {
		t.Fatal("2000 sockets under a 1024 limit is short, and the lab has to say so")
	}
	for _, want := range []string{"1024", "2064", "ulimit -n"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the warning does not mention %q: %q", want, msg)
		}
	}
}

func TestReadOnAFileWithoutTheRowIsZeroAndNoError(t *testing.T) {
	got, err := limits.Read(write(t, "Limit                     Soft Limit           Hard Limit           Units\n"))
	if err != nil {
		t.Fatalf("a limits file without the row is not an error here: %v", err)
	}
	if got.SoftOpenFiles != 0 {
		t.Errorf("Read() = %+v, want zero", got)
	}
	if _, short := got.Short(100000); short {
		t.Error("an unknown limit must not produce a warning about a limit")
	}
}
