// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFilterCLearLatest tests the MsgFilterClear API against the latest
// protocol version.
func TestFilterClearLatest(t *testing.T) {
	pver := ProtocolVersion
	msg := NewMsgFilterClear()

	require.Equal(t, "filterclear", msg.Command())
	require.Equal(t, uint32(0), msg.MaxPayloadLength(pver))
}

// TestFilterClearWire tests the MsgFilterClear wire encode and decode.
func TestFilterClearWire(t *testing.T) {
	msg := NewMsgFilterClear()

	// Encode the message to wire format.
	var buf bytes.Buffer
	err := msg.LatEncode(&buf, ProtocolVersion, BaseEncoding)
	require.NoError(t, err)

	// MsgFilterClear has no payload, so encoded bytes should be empty.
	require.Empty(t, buf.Bytes())

	// Decode the message from wire format.
	var readmsg MsgFilterClear
	rbuf := bytes.NewReader(buf.Bytes())
	err = readmsg.LatDecode(rbuf, ProtocolVersion, BaseEncoding)
	require.NoError(t, err)
}
