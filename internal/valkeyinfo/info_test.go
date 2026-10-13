package valkeyinfo_test

import (
	"testing"

	"github.com/lasttoss/valkey-scale-lab/internal/valkeyinfo"
)

// The fixture is a real INFO reply with the sections this package reads, trimmed. The fields that
// are not read are kept, because the parser has to survive them: a parser that only handles the
// lines it wants is a parser that breaks when the server adds one.
const realInfo = `# Clients
connected_clients:1203
cluster_connections:0
maxclients:1000
client_recent_max_input_buffer:20480
client_recent_max_output_buffer:0
blocked_clients:0
tracking_clients:0

# Stats
total_connections_received:10120
total_commands_processed:884231
instantaneous_ops_per_sec:1204
rejected_connections:0
expired_keys:0

# Commandstats
cmdstat_get:calls=44771,usec=338123,usec_per_call=7.55,rejected_calls=0,failed_calls=0
cmdstat_keys:calls=715,usec=4567899,usec_per_call=6389.37,rejected_calls=0,failed_calls=0
cmdstat_ping:calls=1204,usec=4211,usec_per_call=3.50,rejected_calls=0,failed_calls=0
`

func TestParseReadsTheIncident(t *testing.T) {
	got := valkeyinfo.Parse(realInfo)
	want := valkeyinfo.Info{
		ConnectedClients:         1203,
		MaxClients:               1000,
		TotalConnectionsReceived: 10120,
		KeysCalls:                715,
		KeysUsec:                 4567899,
	}
	if got != want {
		t.Errorf("Parse() = %+v, want %+v", got, want)
	}
}

// TestParseOnAnEmptyReplyIsZeroNotAPanic is the case that arrives on a server configured without
// commandstats, and a lab that panics on a valid server is a lab nobody runs twice.
func TestParseOnAnEmptyReplyIsZeroNotAPanic(t *testing.T) {
	if got := valkeyinfo.Parse("# Clients\nmaxclients:10000\n"); got.ConnectedClients != 0 || got.MaxClients != 10000 {
		t.Errorf("Parse() = %+v", got)
	}
}
