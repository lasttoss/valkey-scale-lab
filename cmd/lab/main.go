// Command lab reproduces the connection incident and measures the fix, against a real Valkey.
//
// It is not a benchmark harness. It is the smallest program that answers two questions with
// numbers instead of adjectives:
//
//	how many connections does one pod open, and what does everyone else's latency do while it
//	walks the keyspace?
//
// Every number it prints comes from the server's own INFO. If the numbers do not have the shape the
// incident had - connections that scale with the match count, an error the server spelled out - the
// command exits non-zero, because a lab that cannot fail is a lab that proves nothing.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/lasttoss/valkey-scale-lab/internal/hotpath"
	"github.com/lasttoss/valkey-scale-lab/internal/keywalk"
	"github.com/lasttoss/valkey-scale-lab/internal/latency"
	"github.com/lasttoss/valkey-scale-lab/internal/limits"
	"github.com/lasttoss/valkey-scale-lab/internal/valkeyinfo"
)

type result struct {
	Mode             string
	Matches          int
	ConnectedBefore  int64
	ConnectedAfter   int64
	MaxClients       int64
	TotalConnections int64
	ClientsInProcess int
	Errors           int
	Refused          int
	FirstError       string
	P50, P99, P999   time.Duration
	Max              time.Duration
	Probes           int
	Sweeps           int
	KeysFound        int
	SweepWall        time.Duration
	KeysCalls        int64
	KeysUsec         int64
}

func main() {
	mode := flag.String("mode", "fixed", "leak (a client per match) or fixed (one client, cached)")
	addr := flag.String("addr", "localhost:6479", "Valkey address")
	db := flag.Int("db", 0, "database number")
	matches := flag.Int("matches", 600, "live matches to represent")
	keys := flag.Int("keys", 0, "keys to create for the KEYS/SCAN sweep (0 = skip)")
	sweeps := flag.Int("sweeps", 0, "full keyspace walks to run (0 = skip)")
	walk := flag.String("walk", "scan", "keys or scan: which walk the sweeps use")
	pool := flag.Int("pool", 16, "connections the shared client is allowed (fixed mode)")
	out := flag.String("out", "", "write the markdown table here")
	finish := flag.Duration("timeout", 120*time.Second, "give up after this long")
	flag.Parse()

	if *mode != "leak" && *mode != "fixed" {
		fmt.Fprintf(os.Stderr, "lab: -mode must be leak or fixed, not %q\n", *mode)
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *finish)
	defer cancel()

	// The file descriptor ceiling is part of this story: a pod that dies of EMFILE does not look
	// like a pod that ran out of memory, and the error it prints names the server, not itself.
	if lim, err := limits.Read("/proc/self/limits"); err == nil {
		if msg, short := lim.Short(*matches); short {
			fmt.Fprintf(os.Stderr, "  warning: %s\n", msg)
		}
	}

	// The monitor is the lab's own instrument: a separate client, a small pool, never the thing
	// being measured. Sampling latency from the client under test measures the client.
	monitor := redis.NewClient(&redis.Options{Addr: *addr, DB: *db, PoolSize: 8, MinIdleConns: 2})
	defer monitor.Close()
	if err := monitor.Ping(ctx).Err(); err != nil {
		fmt.Fprintf(os.Stderr, "lab: cannot reach %s: %v\n", *addr, err)
		os.Exit(1)
	}

	before, err := valkeyinfo.Sample(ctx, monitor)
	if err != nil {
		fmt.Fprintln(os.Stderr, "lab: INFO:", err)
		os.Exit(1)
	}
	if *keys > 0 {
		if err := seed(ctx, monitor, *keys); err != nil {
			fmt.Fprintln(os.Stderr, "lab: seed:", err)
			os.Exit(1)
		}
	}

	sampler := latency.NewSampler(func() error { return monitor.Ping(ctx).Err() })
	sampler.Start()

	res := result{Mode: *mode, Matches: *matches, ConnectedBefore: before.ConnectedClients, MaxClients: before.MaxClients}

	switch *mode {
	case "leak":
		// One client per live match, held open for as long as the match is: this is the incident.
		clients := make([]*redis.Client, 0, *matches)
		for i := 0; i < *matches; i++ {
			c := hotpath.PerMatch(*addr, *db)
			clients = append(clients, c)
			if err := c.Ping(ctx).Err(); err != nil {
				record(&res, err)
				continue
			}
			if err := c.Set(ctx, fmt.Sprintf("lab:match:%d", i), "live", time.Minute).Err(); err != nil {
				record(&res, err)
			}
		}
		res.ClientsInProcess = len(clients)
		defer func() {
			for _, c := range clients {
				_ = c.Close()
			}
		}()
	case "fixed":
		shared := hotpath.NewShared(hotpath.Pool{Size: *pool, MinIdleConns: 2, MaxActiveConns: *pool, IdleTimeout: 5 * time.Minute})
		defer shared.Close()
		c := shared.Client(*addr, *db)
		for i := 0; i < *matches; i++ {
			// The same client, used the way a match would use it. The work is identical to the
			// leak mode's; only the number of sockets is different.
			if err := c.Ping(ctx).Err(); err != nil {
				record(&res, err)
				continue
			}
			if err := c.Set(ctx, fmt.Sprintf("lab:match:%d", i), "live", time.Minute).Err(); err != nil {
				record(&res, err)
			}
		}
		res.ClientsInProcess = shared.Open()
	}

	after, err := valkeyinfo.Sample(ctx, monitor)
	if err != nil {
		fmt.Fprintln(os.Stderr, "lab: INFO:", err)
		os.Exit(1)
	}
	res.ConnectedAfter = after.ConnectedClients
	res.TotalConnections = after.TotalConnectionsReceived - before.TotalConnectionsReceived

	if *sweeps > 0 {
		start := time.Now()
		for i := 0; i < *sweeps; i++ {
			var (
				found []string
				err   error
			)
			if *walk == "keys" {
				found, err = keywalk.Keys(ctx, monitor, "lab:*")
			} else {
				found, err = keywalk.Scan(ctx, monitor, "lab:*", 1000)
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, "lab: sweep:", err)
				os.Exit(1)
			}
			res.KeysFound = len(found)
		}
		res.Sweeps = *sweeps
		res.SweepWall = time.Since(start)
	}

	samples, probeErrors := sampler.Result()
	sampler.Stop()
	res.P50 = latency.Percentile(samples, 0.50)
	res.P99 = latency.Percentile(samples, 0.99)
	res.P999 = latency.Percentile(samples, 0.999)
	if n := len(samples); n > 0 {
		res.Max = samples[n-1]
	}
	res.Probes = len(samples)
	res.Errors += probeErrors

	if final, err := valkeyinfo.Sample(ctx, monitor); err == nil {
		res.KeysCalls = final.KeysCalls - before.KeysCalls
		res.KeysUsec = final.KeysUsec - before.KeysUsec
	}

	printResult(res, *walk)
	if *out != "" {
		if err := writeMarkdown(*out, res, *walk); err != nil {
			fmt.Fprintln(os.Stderr, "lab:", err)
			os.Exit(1)
		}
	}
	if bad := assert(res, *walk); len(bad) > 0 {
		for _, b := range bad {
			fmt.Fprintln(os.Stderr, "  FAIL", b)
		}
		os.Exit(1)
	}
}

// record counts a failure and keeps the first message verbatim. The server's own words are the
// evidence: a lab that summarises them into "connection error" has thrown away the sentence the
// incident was diagnosed with.
func record(r *result, err error) {
	if err == nil {
		return
	}
	r.Errors++
	msg := err.Error()
	if r.FirstError == "" {
		r.FirstError = msg
	}
	// "max number of clients reached" is the message this incident is known by. It is counted
	// separately, and never instead of the others: a dial failure under a full server is the same
	// fact arriving one layer lower.
	if strings.Contains(strings.ToLower(msg), "clients") {
		r.Refused++
	}
}

func seed(ctx context.Context, c *redis.Client, n int) error {
	const batch = 1000
	for start := 0; start < n; start += batch {
		pipe := c.Pipeline()
		end := start + batch
		if end > n {
			end = n
		}
		for i := start; i < end; i++ {
			pipe.Set(ctx, fmt.Sprintf("lab:key:%d", i), "x", time.Hour)
		}
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

func printResult(r result, walk string) {
	fmt.Printf("  %-6s matches=%-5d clients-in-process=%-5d connected %d -> %d (maxclients %d)\n",
		r.Mode, r.Matches, r.ClientsInProcess, r.ConnectedBefore, r.ConnectedAfter, r.MaxClients)
	fmt.Printf("         new connections to the server: %d, errors: %d", r.TotalConnections, r.Errors)
	if r.Errors > 0 {
		fmt.Printf("; %d of them with \"clients\" in the message; the server's words: %s", r.Refused, r.FirstError)
	}
	fmt.Println()
	if r.Sweeps > 0 {
		fmt.Printf("         %s: %d sweeps, %d keys, wall %s, cmdstat_keys calls %d (%.1f ms inside the server)\n",
			walk, r.Sweeps, r.KeysFound, r.SweepWall.Round(time.Millisecond), r.KeysCalls, float64(r.KeysUsec)/1000)
	}
	fmt.Printf("         probe p50 %s  p99 %s  p99.9 %s  max %s  (%d probes)\n",
		r.P50.Round(time.Microsecond), r.P99.Round(time.Microsecond),
		r.P999.Round(time.Microsecond), r.Max.Round(time.Microsecond), r.Probes)
}

// assert is where the lab stops being a report. Each condition is a sentence from the incident; a
// number that does not have that shape means the lab is measuring something else.
func assert(r result, walk string) []string {
	var bad []string
	switch r.Mode {
	case "leak":
		// A client per match shows up as a connection per match - until the server's own ceiling
		// stops it. Both halves are the incident, so both are asserted.
		want := int64(r.Matches)
		if r.MaxClients > 0 && want > r.MaxClients {
			want = r.MaxClients
		}
		if r.ConnectedAfter < want {
			bad = append(bad, fmt.Sprintf("leak mode reached %d connections for %d matches (ceiling %d): a client per match has to show up as a connection per match",
				r.ConnectedAfter, r.Matches, r.MaxClients))
		}
		if r.MaxClients > 0 && int64(r.Matches) > r.MaxClients && r.Errors == 0 {
			bad = append(bad, fmt.Sprintf("the server has maxclients %d and the lab asked for %d connections, but nothing was refused; the reproduction is wrong",
				r.MaxClients, r.Matches))
		}
	case "fixed":
		// The shared client's pool is the ceiling, plus the monitor's own connections, plus slack
		// for a connection being opened while the sample is taken.
		ceiling := int64(r.ClientsInProcess) + 16
		if r.ConnectedAfter > ceiling {
			bad = append(bad, fmt.Sprintf("fixed mode used %d connections with a pool of %d; the ceiling is MaxActiveConns and it is not holding",
				r.ConnectedAfter, r.ClientsInProcess))
		}
		if r.Errors > 0 {
			bad = append(bad, fmt.Sprintf("%d errors with a pool of %d connections: the fix has to complete the same work", r.Errors, r.ClientsInProcess))
		}
	}
	if r.Sweeps > 0 && walk == "scan" && r.KeysCalls != 0 {
		bad = append(bad, fmt.Sprintf("the sweeps said SCAN but the server counted %d KEYS calls", r.KeysCalls))
	}
	if r.Sweeps > 0 && walk == "keys" && r.KeysCalls < int64(r.Sweeps) {
		bad = append(bad, fmt.Sprintf("the sweeps said KEYS but the server counted %d cmdstat_keys calls for %d sweeps", r.KeysCalls, r.Sweeps))
	}
	return bad
}

func writeMarkdown(path string, r result, walk string) error {
	var b strings.Builder
	b.WriteString("| mode | matches | connections on the server | clients in this process | errors | p50 | p99 | p99.9 | max |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|\n")
	fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %s | %s | %s | %s |\n", r.Mode, r.Matches, r.ConnectedAfter,
		r.ClientsInProcess, r.Errors, r.P50.Round(time.Microsecond), r.P99.Round(time.Microsecond),
		r.P999.Round(time.Microsecond), r.Max.Round(time.Microsecond))
	if r.Sweeps > 0 {
		fmt.Fprintf(&b, "\n%s: %d sweeps over %d keys in %s", walk, r.Sweeps, r.KeysFound, r.SweepWall.Round(time.Millisecond))
		if walk == "keys" {
			fmt.Fprintf(&b, ", %.1f ms of that inside the server (cmdstat_keys), and the worst round trip any other client saw was %s",
				float64(r.KeysUsec)/1000, r.Max.Round(10*time.Microsecond))
		} else {
			fmt.Fprintf(&b, ", with cmdstat_keys unchanged (the server never ran KEYS), and the worst round trip any other client saw was %s",
				r.Max.Round(10*time.Microsecond))
		}
		b.WriteString("\n")
	}
	if r.Errors > 0 {
		fmt.Fprintf(&b, "\nserver's words for the refused connections: `%s`\n", r.FirstError)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
