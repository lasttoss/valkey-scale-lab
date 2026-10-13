# GitHub metadata (paste into the repository settings)

**Description**
> One Redis client per match, or one per process: a connection incident reproduced against a real Valkey, with the fix and the numbers - 600 matches held 500 connections and refused 103 of them, one pooled client serves the same 600 with single-digit connections.

**Topics**
valkey, redis, golang, connection-pool, incident-response, load-testing, performance, observability, kubernetes, game-server

**Pinned blurb for the profile README**
> `valkey-scale-lab` - the connection count was a function of the match count, which is why raising `maxclients` was not the fix; the fix is a cache key and a ceiling, measured.
