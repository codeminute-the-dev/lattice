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

func TestFindLatwalletBinary(t *testing.T) {
	// Make PATH lookups fail deterministically.
	t.Setenv("PATH", t.TempDir())

	t.Run("explicit path wins", func(t *testing.T) {
		bin := filepath.Join(t.TempDir(), "custom-latwallet")
		writeFakeBinary(t, bin)
		cfg := &config{LatwalletBin: bin}
		p, src, err := findLatwalletBinary(cfg)
		require.NoError(t, err)
		assert.Equal(t, bin, p)
		assert.Equal(t, srcFlag, src)
	})

	t.Run("explicit path missing errors", func(t *testing.T) {
		cfg := &config{LatwalletBin: filepath.Join(t.TempDir(), "nope")}
		_, _, err := findLatwalletBinary(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--latwalletbin")
	})

	t.Run("found in PATH", func(t *testing.T) {
		dir := t.TempDir()
		writeFakeBinary(t, filepath.Join(dir, "latwallet"))
		t.Setenv("PATH", dir)
		cfg := &config{LatwalletBin: "latwallet"}
		p, src, err := findLatwalletBinary(cfg)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(dir, "latwallet"), p)
		assert.Equal(t, srcPath, src)
	})

	// Binaries in untrusted implicit locations must never be picked up:
	// latwalletcli passes wallet passphrases and seeds to the daemon it runs.
	t.Run("cwd is not searched", func(t *testing.T) {
		dir := t.TempDir()
		writeFakeBinary(t, filepath.Join(dir, "latwallet"))
		writeFakeBinary(t, filepath.Join(dir, "bin", "latwallet"))
		t.Chdir(dir)
		cfg := &config{LatwalletBin: "latwallet"}
		_, _, err := findLatwalletBinary(cfg)
		require.Error(t, err)
	})

	t.Run("executable dir is not searched", func(t *testing.T) {
		exe, err := os.Executable()
		require.NoError(t, err)
		sibling := filepath.Join(filepath.Dir(exe), "latwallet")
		writeFakeBinary(t, sibling)
		t.Cleanup(func() { os.Remove(sibling) })

		cfg := &config{LatwalletBin: "latwallet"}
		_, _, err = findLatwalletBinary(cfg)
		require.Error(t, err)
	})

	t.Run("not found reports remedies", func(t *testing.T) {
		cfg := &config{LatwalletBin: "latwallet"}
		_, _, err := findLatwalletBinary(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not on your $PATH")
		assert.Contains(t, err.Error(), "--latwalletbin")
	})

	t.Run("result is cached", func(t *testing.T) {
		bin := filepath.Join(t.TempDir(), "latwallet")
		writeFakeBinary(t, bin)
		cfg := &config{LatwalletBin: bin}
		_, _, err := findLatwalletBinary(cfg)
		require.NoError(t, err)

		// Remove the file: the cached path must still be returned.
		require.NoError(t, os.Remove(bin))
		p, _, err := findLatwalletBinary(cfg)
		require.NoError(t, err)
		assert.Equal(t, bin, p)
	})
}

func TestIsExecutableFile(t *testing.T) {
	dir := t.TempDir()

	binary := filepath.Join(dir, "binary")
	writeFakeBinary(t, binary)
	assert.True(t, isExecutableFile(binary))

	plain := filepath.Join(dir, "plain")
	require.NoError(t, os.WriteFile(plain, []byte("x"), 0o644))
	assert.False(t, isExecutableFile(plain))

	assert.False(t, isExecutableFile(dir))
	assert.False(t, isExecutableFile(filepath.Join(dir, "missing")))
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "/usr/local/bin/latwallet", "/usr/local/bin/latwallet"},
		{"spaces", "/Users/or/Library/Application Support/Latwallet", "'/Users/or/Library/Application Support/Latwallet'"},
		{"single quote", "it's", `'it'\''s'`},
		{"empty", "", "''"},
		{"tilde", "~/bin/latwallet", "'~/bin/latwallet'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shellQuote(tt.in))
		})
	}
}

func TestSpawnArgs(t *testing.T) {
	t.Run("default appdata mainnet has no args", func(t *testing.T) {
		cfg := &config{AppData: latwalletHomeDir}
		cfg.activeNet = mainNetForTest()
		assert.Empty(t, spawnArgs(cfg))
	})

	t.Run("custom appdata and network are passed", func(t *testing.T) {
		cfg := &config{AppData: "/tmp/custom", TestNet2: true}
		cfg.activeNet = mainNetForTest()
		assert.Equal(t, []string{"--appdata=/tmp/custom", "--testnet2"}, spawnArgs(cfg))
	})
}
