// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPing tests the MsgPing API against the latest protocol version.
func TestPing(t *testing.T) {
	pver := ProtocolVersion

	nonce, err := RandomUint64()
	require.NoError(t, err)

	msg := NewMsgPing(nonce)
	require.Equal(t, nonce, msg.Nonce)
	require.Equal(t, "ping", msg.Command())
	require.Equal(t, uint32(8), msg.MaxPayloadLength(pver))
}

// TestPingWire tests the MsgPing wire encode and decode.
func TestPingWire(t *testing.T) {
	tests := []struct {
		in   MsgPing
		out  MsgPing
		buf  []byte
		pver uint32
	}{
		{
			MsgPing{Nonce: 123123}, // 0x1e0f3
			MsgPing{Nonce: 123123},
			[]byte{0xf3, 0xe0, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00},
			ProtocolVersion,
		},
		{
			MsgPing{Nonce: 456456}, // 0x6f708
			MsgPing{Nonce: 456456},
			[]byte{0x08, 0xf7, 0x06, 0x00, 0x00, 0x00, 0x00, 0x00},
			ProtocolVersion + 1,
		},
	}

	for i, test := range tests {
		var buf bytes.Buffer
		err := test.in.LatEncode(&buf, test.pver, BaseEncoding)
		require.NoError(t, err, "test #%d", i)
		require.Equal(t, test.buf, buf.Bytes(), "test #%d", i)

		var msg MsgPing
		rbuf := bytes.NewReader(test.buf)
		err = msg.LatDecode(rbuf, test.pver, BaseEncoding)
		require.NoError(t, err, "test #%d", i)
		require.Equal(t, test.out, msg, "test #%d", i)
	}
}

// TestPingWireErrors performs negative tests against wire encode and decode
// of MsgPing to confirm error paths work correctly.
func TestPingWireErrors(t *testing.T) {
	pver := ProtocolVersion
	pingMsg := &MsgPing{Nonce: 123123}
	pingEncoded := []byte{0xf3, 0xe0, 0x01, 0x00}

	// Force error with max=2.
	w := newFixedWriter(2)
	err := pingMsg.LatEncode(w, pver, BaseEncoding)
	require.ErrorIs(t, err, io.ErrShortWrite)

	var msg MsgPing
	r := newFixedReader(2, pingEncoded)
	err = msg.LatDecode(r, pver, BaseEncoding)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
}
