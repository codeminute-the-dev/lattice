package dnsseed

import (
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    string
		valid     bool
		network   string
		interval  time.Duration
		bootstrap []string
		ttl       uint32
		maxAns    uint32
	}{
		{
			name:   "bare dnsseed",
			config: `dnsseed`,
			valid:  false,
		},
		{
			name:   "empty block",
			config: `dnsseed { }`,
			valid:  false,
		},
		{
			name:   "network without value",
			config: `dnsseed { network }`,
			valid:  false,
		},
		{
			name:     "mainnet defaults",
			config:   `dnsseed { network mainnet }`,
			valid:    true,
			network:  "mainnet",
			interval: defaultUpdateInterval,
			ttl:      defaultTTL,
			maxAns:   defaultMaxAnswers,
		},
		{
			name:   "bootstrap_peers without values",
			config: "dnsseed {\n  network testnet\n  crawl_interval 15s\n  bootstrap_peers\n}",
			valid:  false,
		},
		{
			name:   "crawl_interval without value",
			config: "dnsseed {\n  network testnet\n  crawl_interval\n  bootstrap_peers 127.0.0.1:44110\n}",
			valid:  false,
		},
		{
			name:      "testnet with custom interval and peers",
			config:    "dnsseed {\n  network testnet\n  crawl_interval 15s\n  bootstrap_peers 127.0.0.1:44110\n}",
			valid:     true,
			network:   "testnet",
			interval:  15 * time.Second,
			bootstrap: []string{"127.0.0.1:44110"},
			ttl:       defaultTTL,
			maxAns:    defaultMaxAnswers,
		},
		{
			name:   "unknown option rejected",
			config: "dnsseed {\n  network testnet\n  bootstrap_peers 127.0.0.1:44110\n  boop snoot\n}",
			valid:  false,
		},
		{
			name:      "mainnet full config",
			config:    "dnsseed {\n  network mainnet\n  crawl_interval 30m\n  bootstrap_peers 127.0.0.1:44108 127.0.0.2:44108\n  record_ttl 300\n  max_answers 10\n}",
			valid:     true,
			network:   "mainnet",
			interval:  30 * time.Minute,
			bootstrap: []string{"127.0.0.1:44108", "127.0.0.2:44108"},
			ttl:       300,
			maxAns:    10,
		},
		{
			name:     "regtest network accepted",
			config:   `dnsseed { network regtest }`,
			valid:    true,
			network:  "regtest",
			interval: defaultUpdateInterval,
			ttl:      defaultTTL,
			maxAns:   defaultMaxAnswers,
		},
		{
			name:   "invalid network rejected",
			config: `dnsseed { network fakenet }`,
			valid:  false,
		},
		{
			name:   "bad ttl rejected",
			config: "dnsseed {\n  network mainnet\n  record_ttl -1\n}",
			valid:  false,
		},
		{
			name:   "bad max_answers rejected",
			config: "dnsseed {\n  network mainnet\n  max_answers 999\n}",
			valid:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := caddy.NewTestController("dns", tt.config)
			opts, err := parseConfig(c)

			if !tt.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, tt.network, opts.networkName)
			assert.Equal(t, tt.interval, opts.updateInterval)
			assert.Equal(t, tt.ttl, opts.recordTTL)
			assert.Equal(t, tt.maxAns, opts.maxAnswers)

			for i, s := range tt.bootstrap {
				assert.Equal(t, s, opts.bootstrapPeers[i])
			}
		})
	}
}
