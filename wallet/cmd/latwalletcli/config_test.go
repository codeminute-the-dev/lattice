// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScrapeLatwalletConf(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    latwalletConfValues
	}{
		{
			name:    "latwallet style options",
			content: "[Application Options]\nusername=alice\npassword=hunter2\n",
			want:    latwalletConfValues{username: "alice", password: "hunter2"},
		},
		{
			name:    "latticed style aliases",
			content: "rpcuser=bob\nrpcpass=secret\n",
			want:    latwalletConfValues{username: "bob", password: "secret"},
		},
		{
			name:    "commented options ignored",
			content: "; username=nope\n;password=nope\nusername=real\npassword=pw\n",
			want:    latwalletConfValues{username: "real", password: "pw"},
		},
		{
			name:    "noservertls enabled",
			content: "username=u\npassword=p\nnoservertls=1\n",
			want:    latwalletConfValues{username: "u", password: "p", noServerTLS: true},
		},
		{
			name:    "noservertls disabled",
			content: "noservertls=0\n",
			want:    latwalletConfValues{},
		},
		{
			name:    "leading whitespace",
			content: "  username=indented\n\tpassword=tabbed\n",
			want:    latwalletConfValues{username: "indented", password: "tabbed"},
		},
		{
			name:    "empty file",
			content: "",
			want:    latwalletConfValues{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "latwallet.conf")
			require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o600))
			assert.Equal(t, tt.want, scrapeLatwalletConf(path))
		})
	}
}

func TestScrapeLatwalletConfMissingFile(t *testing.T) {
	got := scrapeLatwalletConf(filepath.Join(t.TempDir(), "does-not-exist.conf"))
	assert.Equal(t, latwalletConfValues{}, got)
}

func TestRescrapeConf(t *testing.T) {
	cfg := &config{AppData: t.TempDir()}
	cfg.activeNet = mainNetForTest()
	require.NoError(t, os.WriteFile(cfg.latwalletConfPath(),
		[]byte("username=fresh\npassword=secret\nnoservertls=1\n"), 0o600))

	cfg.rescrapeConf()

	assert.Equal(t, "fresh", cfg.RPCUser)
	assert.Equal(t, "secret", cfg.RPCPass)
	assert.True(t, cfg.NoTLS)
	assert.Equal(t, "found", cfg.src.conf)
	assert.Equal(t, "latwallet.conf (auto-provisioned)", cfg.src.creds)
}
