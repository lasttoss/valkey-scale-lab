# Measurements

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

### 600 live matches, one pooled client

| mode | matches | connections on the server | clients in this process | errors | p50 | p99 | p99.9 | max |
|---|---|---|---|---|---|---|---|---|
| fixed | 600 | 7 | 1 | 0 | 29µs | 103µs | 1.908ms | 2.263ms |

### `KEYS` over 200,000 keys, 20 walks

| mode | matches | connections on the server | clients in this process | errors | p50 | p99 | p99.9 | max |
|---|---|---|---|---|---|---|---|---|
| fixed | 50 | 7 | 1 | 0 | 29µs | 391µs | 16.055ms | 18.685ms |

keys: 20 sweeps over 200600 keys in 571ms, 295.0 ms of that inside the server (cmdstat_keys), and the worst round trip any other client saw was 18.68ms

### 300 live matches, a client per match

| mode | matches | connections on the server | clients in this process | errors | p50 | p99 | p99.9 | max |
|---|---|---|---|---|---|---|---|---|
| leak | 300 | 303 | 300 | 0 | 36µs | 165µs | 1.041ms | 1.58ms |

### 600 live matches, a client per match (past the ceiling)

| mode | matches | connections on the server | clients in this process | errors | p50 | p99 | p99.9 | max |
|---|---|---|---|---|---|---|---|---|
| leak | 600 | 500 | 600 | 103 | 29µs | 57µs | 255µs | 4.352ms |

server's words for the refused connections: `EOF`

### `SCAN` over the same 200,000 keys, 20 walks

| mode | matches | connections on the server | clients in this process | errors | p50 | p99 | p99.9 | max |
|---|---|---|---|---|---|---|---|---|
| fixed | 50 | 6 | 1 | 0 | 218µs | 840µs | 1.831ms | 3.786ms |

scan: 20 sweeps over 200600 keys in 1.371s, with cmdstat_keys unchanged (the server never ran KEYS), and the worst round trip any other client saw was 3.79ms


## What the numbers do not say

- `mode=leak` is a lab, not a leak detector: the clients are held open on purpose, because that is
  what a live match did. The real incident reached 10,120 connections from one pod over hours, not
  500 in one run.
- The `EOF` under the refused connections is the client's view. The server logs the sentence this
  incident is known by (`max number of clients reached`); the socket sees a close. Both are true,
  and a dashboard that only graphs the client's error text will call this "connection reset".
- Latency here is loopback on one machine. The p99.9 is the interesting column: `KEYS` stalls other
  clients for the length of the scan, and that stall is what a player feels as a rubber band.
