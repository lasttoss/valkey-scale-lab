# Production notes: connections, limits, and the wall that is not the limit

These are the notes I would hand to the next person on call for a game backend whose Redis is
answering `max number of clients reached`. The lab in this repository reproduces the mechanism
(`make measure`); the notes are what surrounds it on a real host.

Everything here is from systems I have run. Numbers that belong to a specific incident are the
numbers that incident produced, with the client's identity and the game's name removed.

## 1. The incident this repository reproduces

A live title, one pod, one Redis (Valkey) shared by every match on it:

- the pod was holding **10,120 client connections** to one Redis, and the number grew with the
  number of live matches;
- Redis answered **`max number of clients reached`** and the game had no new lobbies;
- the metric that explained it - `connected_clients` - had no panel, so the first hypothesis was
  "Redis is overloaded". It was not: it was serving a normal command rate to an abnormal number of
  sockets.

The cause was structural, not a leak: **the match handler created a `redis.Client`**. Every client
owns a connection pool, every pool opens sockets on first use, and a pod with ten thousand live
matches had ten thousand pools. Nothing was unreachable garbage; the count was simply a function of
the match count.

`maxclients` was not the bug. Raising it moves the wall; it does not remove it. The three changes
that removed it:

1. **One client per process**, cached by `(address, database)`. The database is part of the key:
   two databases on one address are two keyspaces and one server.
2. **An explicit pool.** `PoolSize` is not a ceiling - go-redis allocates in excess of it under
   load - so `MaxActiveConns` is the number that bounds a pod.
3. **`SCAN`, not `KEYS`,** for the keyspace walks that admin endpoints and tooling do.

The lab in this repository measures exactly these three, and the measurement that convinced me was
not the connection count. It was the **p99.9 of an unrelated client** during the walk: 18.7 ms with
`KEYS`, 3.8 ms with `SCAN` over the same 200,600 keys.

## 2. The Unix side of the same problem

A connection incident is usually two limits meeting: the server's `maxclients` and the client
host's ability to hold sockets.

| Knob | Where | What it does | What it did to me |
|---|---|---|---|
| `ulimit -n` (soft) | process / container | maximum open files per process | the usual reason a pod dies of EMFILE while the server is fine; 1024 is still a common soft default |
| `maxclients` | Redis config | accepted client connections | the wall in §1, and not the bug |
| `net.core.somaxconn` | kernel | accept queue length | Redis logs a warning when `tcp-backlog` (511) exceeds it; old images default to 128 |
| `net.ipv4.tcp_max_syn_backlog` | kernel | half-open connections | matters on the same overload that fills the accept queue |
| `net.ipv4.ip_local_port_range` | kernel | outbound ephemeral ports | a hard ceiling on how many *outbound* connections one client host can make; ~28k, per (source, destination) pair |
| `net.ipv4.tcp_tw_reuse` | kernel | reuse TIME_WAIT sockets | the difference between "sockets closed" and "ports available" when a pod churns connections |
| `nf_conntrack_count` / `nf_conntrack_max` | kernel | tracked connections | saw a node at 13.6k of 262k while debugging this class of problem; it is a limit that is not in the Redis config |
| `vm.overcommit_memory` | kernel | fork for background saves | Redis warns about it at startup; ignoring it is how a save turns into a stall |
| transparent huge pages | kernel | memory mapping | Redis warns about it too, and the warning is about latency spikes, not memory |
| CPU limit (cgroup) | Kubernetes | throttling | a Redis at its CPU limit does not look slow in `top`; it looks slow in `container_cpu_cfs_throttled_periods_total` |

The rule I keep: **if a limit exists, it is in the incident**. The postmortem that says "we raised
`maxclients`" is a postmortem that will be written again.

## 3. What to panel

Five panels, in the order I would look at them:

1. `connected_clients` per Redis, **per client pod** (`CLIENT LIST` grouped by address). The number
   that was invisible in §1.
2. `total_connections_received` rate - not the current count. A client that opens and closes is
   normal; a client that opens and holds is the incident.
3. `rejected_connections` and `blocked_clients`.
4. `container_cpu_cfs_throttled_periods_total` for the Redis container.
5. `nf_conntrack_count / nf_conntrack_max` for the node.

And one command that ends the argument: `CLIENT LIST` piped into a per-IP count. Two lines of
`awk` answer "who is holding ten thousand sockets" while a dashboard is still being built.

## 4. Where the SQL half of this lives

This repository is deliberately about one server and its connections. The SQL side of the same
interview question - keyset pagination, an append-only audit table, an idempotent event worker - is
in `liveops-console` and `nakama-migration-kit`, because those are the repositories where a
database does real work. A second container in this lab's compose file that no measurement touches
would be a container that costs every run and teaches nothing.
