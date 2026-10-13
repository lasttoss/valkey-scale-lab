// Package hotpath holds the two ways a game server gets a connection to Redis, side by side,
// because the difference between them is the incident this repository exists to reproduce.
//
// The incident: a match server created a redis.Client inside the match handler. Nothing leaked in
// the sense a leak detector would see - every client was eventually garbage, and the GC did its
// job. What grew was the number of *sockets*: each client owns a pool, each pool opens connections
// on first use, and a pod running ten thousand live matches had ten thousand pools. Redis answered
// "max number of clients reached", and the metric that explained it was `connected_clients`, which
// nobody had a panel for.
package hotpath

import (
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Pool is a deliberate number, not a default. go-redis defaults to 10 * GOMAXPROCS connections per
// client, which is a reasonable default for an application and an unreasonable one for a pod that
// creates a client per match: the default multiplies by the match count.
type Pool struct {
	Size           int
	MinIdleConns   int
	MaxActiveConns int
	IdleTimeout    time.Duration
}

// DefaultPool is what a process-wide client should look like: small, kept warm, and recycled, so
// that a restart of Redis does not leave the client holding a hundred dead sockets.
func DefaultPool() Pool {
	return Pool{Size: 16, MinIdleConns: 4, MaxActiveConns: 16, IdleTimeout: 5 * time.Minute}
}

// PerMatch is the code the incident was made of, kept because a lab that only contains the fix
// cannot show what the fix is for. Every call returns a new client, and Every call is what the
// original handler did once per match.
func PerMatch(addr string, db int) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: addr, DB: db})
}

// Shared is the fix: one client per (address, database), created once, reused for the process's
// whole life. The database is part of the key on purpose - two databases on one address are two
// keyspaces and one server, and a cache keyed by address alone would hand a caller the wrong one.
type Shared struct {
	pool  Pool
	mu    sync.Mutex
	cache map[string]*redis.Client
}

func NewShared(pool Pool) *Shared {
	return &Shared{pool: pool, cache: make(map[string]*redis.Client)}
}

// Client returns the process-wide client for this address and database, creating it the first time.
func (s *Shared) Client(addr string, db int) *redis.Client {
	k := key(addr, db)
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.cache[k]; ok {
		return c
	}
	c := redis.NewClient(&redis.Options{
		Addr:         addr,
		DB:           db,
		PoolSize:     s.pool.Size,
		MinIdleConns: s.pool.MinIdleConns,
		// PoolSize is not a ceiling: go-redis allocates in excess of it when the pool is busy.
		// MaxActiveConns is the ceiling, and a pod that must not exceed a hundred sockets to
		// Redis needs this line, not the one above it.
		MaxActiveConns:  s.pool.MaxActiveConns,
		ConnMaxIdleTime: s.pool.IdleTimeout,
	})
	s.cache[k] = c
	return c
}

// Open is the number of clients this process is holding. It is the number that was 10,120.
func (s *Shared) Open() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.cache)
}

// Close releases every client it created. A shared client that is never closed is a lab that
// cannot measure the same server twice.
func (s *Shared) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var first error
	for k, c := range s.cache {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
		delete(s.cache, k)
	}
	return first
}

func key(addr string, db int) string {
	// A separator a hostname cannot contain, so "a:1" + db 2 cannot collide with "a:12" + db 0.
	return addr + "|" + itoa(db)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
