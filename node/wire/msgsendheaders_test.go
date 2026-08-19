// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSendHeaders tests the MsgSendHeaders API against the latest protocol version.
func TestSendHeaders(t *testing.T) {
	pver := ProtocolVersion
	enc := BaseEncoding

	msg := NewMsgSendHeaders()
	require.Equal(t, "sendheaders", msg.Command())
	require.Equal(t, uint32(0), msg.MaxPayloadLength(pver))

	// Test encode with latest protocol version.
	var buf bytes.Buffer
	err := msg.LatEncode(&buf, pver, enc)
	require.NoError(t, err)

	// Test decode with latest protocol version.
	readmsg := NewMsgSendHeaders()
	err = readmsg.LatDecode(&buf, pver, enc)
	require.NoError(t, err)
}

// TestSendHeadersWire tests the MsgSendHeaders wire encode and decode.
func TestSendHeadersWire(t *testing.T) {
	msg := NewMsgSendHeaders()

	var buf bytes.Buffer
	err := msg.LatEncode(&buf, ProtocolVersion, BaseEncoding)
	require.NoError(t, err)
	require.Empty(t, buf.Bytes())

	var decoded MsgSendHeaders
	rbuf := bytes.NewReader(buf.Bytes())
	err = decoded.LatDecode(rbuf, ProtocolVersion, BaseEncoding)
	require.NoError(t, err)
	require.Equal(t, msg, &decoded)
}
