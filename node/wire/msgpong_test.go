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

// TestPongLatest tests the MsgPong API against the latest protocol version.
func TestPongLatest(t *testing.T) {
	enc := BaseEncoding
	pver := ProtocolVersion

	nonce, err := RandomUint64()
	require.NoError(t, err)

	msg := NewMsgPong(nonce)
	require.Equal(t, nonce, msg.Nonce)
	require.Equal(t, "pong", msg.Command())
	require.Equal(t, uint32(8), msg.MaxPayloadLength(pver))

	// Test encode with latest protocol version.
	var buf bytes.Buffer
	err = msg.LatEncode(&buf, pver, enc)
	require.NoError(t, err)

	// Test decode with latest protocol version.
	readmsg := NewMsgPong(0)
	err = readmsg.LatDecode(&buf, pver, enc)
	require.NoError(t, err)

	require.Equal(t, msg.Nonce, readmsg.Nonce)
}

// TestPongWire tests the MsgPong wire encode and decode.
func TestPongWire(t *testing.T) {
	tests := []struct {
		in   MsgPong
		out  MsgPong
		buf  []byte
		pver uint32
	}{
		{
			MsgPong{Nonce: 123123}, // 0x1e0f3
			MsgPong{Nonce: 123123},
			[]byte{0xf3, 0xe0, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00},
			ProtocolVersion,
		},
		{
			MsgPong{Nonce: 456456}, // 0x6f708
			MsgPong{Nonce: 456456},
			[]byte{0x08, 0xf7, 0x06, 0x00, 0x00, 0x00, 0x00, 0x00},
			ProtocolVersion + 1,
		},
	}

	for i, test := range tests {
		var buf bytes.Buffer
		err := test.in.LatEncode(&buf, test.pver, BaseEncoding)
		require.NoError(t, err, "test #%d", i)
		require.Equal(t, test.buf, buf.Bytes(), "test #%d", i)

		var msg MsgPong
		rbuf := bytes.NewReader(test.buf)
		err = msg.LatDecode(rbuf, test.pver, BaseEncoding)
		require.NoError(t, err, "test #%d", i)
		require.Equal(t, test.out, msg, "test #%d", i)
	}
}

// TestPongWireErrors performs negative tests against wire encode and decode
// of MsgPong to confirm error paths work correctly.
func TestPongWireErrors(t *testing.T) {
	pver := ProtocolVersion

	basePong := NewMsgPong(123123) // 0x1e0f3
	basePongEncoded := []byte{
		0xf3, 0xe0, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00,
	}

	// Force error in nonce with max=0.
	w := newFixedWriter(0)
	err := basePong.LatEncode(w, pver, BaseEncoding)
	require.ErrorIs(t, err, io.ErrShortWrite)

	var msg MsgPong
	r := newFixedReader(0, basePongEncoded)
	err = msg.LatDecode(r, pver, BaseEncoding)
	require.ErrorIs(t, err, io.EOF)
}
