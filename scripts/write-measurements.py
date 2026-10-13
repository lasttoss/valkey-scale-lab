#!/usr/bin/env python3
"""Assemble the measured tables into docs/measurements.md.

The tables are produced by `go run ./cmd/lab`, never typed: a number in a README that no command
reproduces is a number that stops being true silently. This script adds the framing and nothing
else, because the moment it computes a figure of its own it stops being a transcription.
"""
import pathlib
import sys

HEAD = """# Measurements

Reproduced with `make measure` against `valkey/valkey:8-alpine` with `--maxclients 500`, from a
single client process on the same machine. The columns are the server's own `INFO` (`clients`,
`stats`, `commandstats`) and the round-trip time of one other client that does nothing but `PING`
while the workload runs.

The two halves of the incident, and the two halves of the fix:

1. **A client per match** (`mode=leak`). Connections are a function of the match count: 300 matches,
   304 connections. Past the server's ceiling the connection is refused.
2. **One client, pooled** (`mode=fixed`). The same 600 matches against the same server, with the
   connection count staying at the pool's size.

The third and fourth tables are the other half of the same incident: the call that walks the
keyspace. `KEYS` is O(N) on a server that serves one command at a time, so the number that matters
is not its own duration - it is the worst round trip everybody else saw while it ran.

"""

OUTRO = """
## What the numbers do not say

- `mode=leak` is a lab, not a leak detector: the clients are held open on purpose, because that is
  what a live match did. The real incident reached 10,120 connections from one pod over hours, not
  500 in one run.
- The `EOF` under the refused connections is the client's view. The server logs the sentence this
  incident is known by (`max number of clients reached`); the socket sees a close. Both are true,
  and a dashboard that only graphs the client's error text will call this "connection reset".
- Latency here is loopback on one machine. The p99.9 is the interesting column: `KEYS` stalls other
  clients for the length of the scan, and that stall is what a player feels as a rubber band.
"""


def main() -> int:
    files = [pathlib.Path(a) for a in sys.argv[1:]]
    if not files:
        print("usage: write-measurements.py <table.md>...", file=sys.stderr)
        return 2
    body = [HEAD]
    # The order is the argument's order, written down here, because the incident has a narrative:
    # the leak, the wall, the fix, and then the other half of the same incident.
    order = ["lab-leak300.md", "lab-leak600.md", "lab-fixed300.md", "lab-fixed600.md", "lab-keys.md", "lab-scan.md"]
    titles = {
        "lab-leak300.md": "### 300 live matches, a client per match",
        "lab-leak600.md": "### 600 live matches, a client per match (past the ceiling)",
        "lab-fixed300.md": "### 300 live matches, one pooled client",
        "lab-fixed600.md": "### 600 live matches, one pooled client",
        "lab-keys.md": "### `KEYS` over 200,000 keys, 20 walks",
        "lab-scan.md": "### `SCAN` over the same 200,000 keys, 20 walks",
    }
    by_name = {f.name: f for f in files}
    ordered = [by_name[n] for n in order if n in by_name]
    ordered += [f for f in files if f.name not in order]
    for f in ordered:
        body.append(titles.get(f.name, f"### {f.name}") + "\n\n")
        body.append(f.read_text().rstrip() + "\n\n")
    body.append(OUTRO)
    sys.stdout.write("".join(body))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
