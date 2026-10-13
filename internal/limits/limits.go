// Package limits reads the one line of /proc/self/limits that a connection incident usually has in
// it: the file descriptor ceiling. It is here because "we would have seen it in the dashboard" is
// false for a pod that dies of EMFILE - the process does not run out of memory, it runs out of
// handles, and the error it prints mentions the server, not itself.
package limits

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type Limits struct {
	SoftOpenFiles uint64
	HardOpenFiles uint64
}

// Read parses the Max open files row of /proc/self/limits. On a system without that file the zero
// value comes back with an error, and the caller decides whether it cares.
func Read(path string) (Limits, error) {
	f, err := os.Open(path)
	if err != nil {
		return Limits{}, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "Max open files") {
			continue
		}
		fields := strings.Fields(line)
		// "Max open files            1024                 1048576              files"
		if len(fields) < 5 {
			return Limits{}, nil
		}
		soft, err1 := strconv.ParseUint(fields[3], 10, 64)
		hard, err2 := strconv.ParseUint(fields[4], 10, 64)
		if err1 != nil || err2 != nil {
			return Limits{}, nil
		}
		return Limits{SoftOpenFiles: soft, HardOpenFiles: hard}, nil
	}
	return Limits{}, sc.Err()
}

// Short reports whether n sockets fit under the soft limit with room for the rest of the process.
// It returns a sentence rather than a bool because the caller prints it, and a number with a reason
// attached is the difference between a warning that is read and one that is scrolled past.
func (l Limits) Short(n int) (string, bool) {
	if l.SoftOpenFiles == 0 {
		return "", false
	}
	need := uint64(n) + 64
	if need > l.SoftOpenFiles {
		return "this process may open " + strconv.FormatUint(l.SoftOpenFiles, 10) +
			" files (soft limit) and this run wants " + strconv.FormatUint(need, 10) +
			": raise it with `ulimit -n`, or the lab will measure the host's limits instead of the server's", true
	}
	return "", false
}
