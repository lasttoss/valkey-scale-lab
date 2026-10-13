// Package valkeyinfo reads the three lines of INFO that explain a connection incident, and refuses
// to guess at the rest. A dashboard that graphs `connected_clients` is worth more than a dashboard
// that graphs everything, because the second one is not read.
package valkeyinfo

import (
	"context"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

type Info struct {
	ConnectedClients         int64
	MaxClients               int64
	TotalConnectionsReceived int64
	KeysCalls                int64 // cmdstat_keys: how many times KEYS was called
	KeysUsec                 int64 // and what the server spent inside it, microseconds
}

// Parse reads the sections of an INFO reply. Unknown lines are ignored on purpose: this struct is
// the shape of one incident, not a model of the server.
func Parse(text string) Info {
	var out Info
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch name {
		case "connected_clients":
			out.ConnectedClients = atoi(value)
		case "maxclients":
			out.MaxClients = atoi(value)
		case "total_connections_received":
			out.TotalConnectionsReceived = atoi(value)
		case "cmdstat_keys":
			// cmdstat_keys:calls=715,usec=4567899,usec_per_call=6389.37,...
			for _, field := range strings.Split(value, ",") {
				k, v, ok := strings.Cut(field, "=")
				if !ok {
					continue
				}
				switch k {
				case "calls":
					out.KeysCalls = atoi(v)
				case "usec":
					out.KeysUsec = atoi(v)
				}
			}
		}
	}
	return out
}

func atoi(s string) int64 {
	// Redis writes whole numbers for these fields; a value it writes differently is not a number
	// this package is willing to invent a meaning for.
	if i := strings.IndexAny(s, "."); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// Sample reads the sections this package understands from a live server.
func Sample(ctx context.Context, c *redis.Client) (Info, error) {
	text, err := c.Info(ctx, "clients", "stats", "commandstats").Result()
	if err != nil {
		return Info{}, err
	}
	return Parse(text), nil
}
