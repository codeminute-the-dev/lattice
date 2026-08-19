// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"io"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFeeFilterLatest tests the MsgFeeFilter API against the latest protocol version.
func TestFeeFilterLatest(t *testing.T) {
	pver := ProtocolVersion

	minfee := rand.Int63()
	msg := NewMsgFeeFilter(minfee)
	require.Equal(t, minfee, msg.MinFee)
	require.Equal(t, "feefilter", msg.Command())
	require.Equal(t, uint32(8), msg.MaxPayloadLength(pver))

	// Test encode with latest protocol version.
	var buf bytes.Buffer
	err := msg.LatEncode(&buf, pver, BaseEncoding)
	require.NoError(t, err)

	// Test decode with latest protocol version.
	readmsg := NewMsgFeeFilter(0)
	err = readmsg.LatDecode(&buf, pver, BaseEncoding)
	require.NoError(t, err)

	require.Equal(t, msg.MinFee, readmsg.MinFee)
}

// TestFeeFilterWire tests the MsgFeeFilter wire encode and decode.
func TestFeeFilterWire(t *testing.T) {
	tests := []struct {
		in   MsgFeeFilter
		out  MsgFeeFilter
		buf  []byte
		pver uint32
	}{
		{
			MsgFeeFilter{MinFee: 123123}, // 0x1e0f3
			MsgFeeFilter{MinFee: 123123},
			[]byte{0xf3, 0xe0, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00},
			ProtocolVersion,
		},
		{
			MsgFeeFilter{MinFee: 456456}, // 0x6f708
			MsgFeeFilter{MinFee: 456456},
			[]byte{0x08, 0xf7, 0x06, 0x00, 0x00, 0x00, 0x00, 0x00},
			ProtocolVersion,
		},
	}

	for i, test := range tests {
		var buf bytes.Buffer
		err := test.in.LatEncode(&buf, test.pver, BaseEncoding)
		require.NoError(t, err, "test #%d", i)
		require.Equal(t, test.buf, buf.Bytes(), "test #%d", i)

		var msg MsgFeeFilter
		rbuf := bytes.NewReader(test.buf)
		err = msg.LatDecode(rbuf, test.pver, BaseEncoding)
		require.NoError(t, err, "test #%d", i)
		require.Equal(t, test.out, msg, "test #%d", i)
	}
}

// TestFeeFilterWireErrors performs negative tests against wire encode and decode
// of MsgFeeFilter to confirm error paths work correctly.
func TestFeeFilterWireErrors(t *testing.T) {
	pver := ProtocolVersion

	baseFeeFilter := NewMsgFeeFilter(123123) // 0x1e0f3
	baseFeeFilterEncoded := []byte{
		0xf3, 0xe0, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00,
	}

	// Force error in minfee with max=0.
	w := newFixedWriter(0)
	err := baseFeeFilter.LatEncode(w, pver, BaseEncoding)
	require.ErrorIs(t, err, io.ErrShortWrite)

	var msg MsgFeeFilter
	r := newFixedReader(0, baseFeeFilterEncoded)
	err = msg.LatDecode(r, pver, BaseEncoding)
	require.ErrorIs(t, err, io.EOF)
}
