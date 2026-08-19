// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSendAddrV2 tests the MsgSendAddrV2 API against the latest protocol version.
func TestSendAddrV2(t *testing.T) {
	pver := ProtocolVersion
	enc := BaseEncoding

	msg := NewMsgSendAddrV2()
	require.Equal(t, "sendaddrv2", msg.Command())
	require.Equal(t, uint32(0), msg.MaxPayloadLength(pver))

	// Test encode with latest protocol version.
	var buf bytes.Buffer
	err := msg.LatEncode(&buf, pver, enc)
	require.NoError(t, err)

	// Test decode with latest protocol version.
	readmsg := NewMsgSendAddrV2()
	err = readmsg.LatDecode(&buf, pver, enc)
	require.NoError(t, err)
}

// TestSendAddrV2Wire tests the MsgSendAddrV2 wire encode and decode.
func TestSendAddrV2Wire(t *testing.T) {
	msg := NewMsgSendAddrV2()

	var buf bytes.Buffer
	err := msg.LatEncode(&buf, ProtocolVersion, BaseEncoding)
	require.NoError(t, err)
	require.Empty(t, buf.Bytes())

	var decoded MsgSendAddrV2
	rbuf := bytes.NewReader(buf.Bytes())
	err = decoded.LatDecode(rbuf, ProtocolVersion, BaseEncoding)
	require.NoError(t, err)
	require.Equal(t, msg, &decoded)
}
