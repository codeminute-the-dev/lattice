// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemPool(t *testing.T) {
	pver := ProtocolVersion
	enc := BaseEncoding
	msg := NewMsgMemPool()

	require.Equal(t, "mempool", msg.Command())
	require.Equal(t, uint32(0), msg.MaxPayloadLength(pver))

	// Test encode with latest protocol version.
	var buf bytes.Buffer
	err := msg.LatEncode(&buf, pver, enc)
	require.NoError(t, err)

	// Test decode with latest protocol version.
	readmsg := NewMsgMemPool()
	err = readmsg.LatDecode(&buf, pver, enc)
	require.NoError(t, err)
}
