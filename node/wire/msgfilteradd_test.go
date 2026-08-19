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

// TestFilterAddLatest tests the MsgFilterAdd API against the latest protocol version.
func TestFilterAddLatest(t *testing.T) {
	enc := BaseEncoding
	pver := ProtocolVersion

	data := []byte{0x01, 0x02}
	msg := NewMsgFilterAdd(data)

	require.Equal(t, "filteradd", msg.Command())
	require.Equal(t, uint32(523), msg.MaxPayloadLength(pver))

	// Test encode with latest protocol version.
	var buf bytes.Buffer
	err := msg.LatEncode(&buf, pver, enc)
	require.NoError(t, err)

	// Test decode with latest protocol version.
	var readmsg MsgFilterAdd
	err = readmsg.LatDecode(&buf, pver, enc)
	require.NoError(t, err)

	require.Equal(t, msg.Data, readmsg.Data)
}

// TestFilterAddMaxDataSize tests the MsgFilterAdd API maximum data size.
func TestFilterAddMaxDataSize(t *testing.T) {
	data := bytes.Repeat([]byte{0xff}, 521)
	msg := NewMsgFilterAdd(data)

	// Encode with latest protocol version should fail.
	var buf bytes.Buffer
	err := msg.LatEncode(&buf, ProtocolVersion, LatestEncoding)
	require.Error(t, err)

	// Decode with latest protocol version should fail.
	readbuf := bytes.NewReader(data)
	err = msg.LatDecode(readbuf, ProtocolVersion, LatestEncoding)
	require.Error(t, err)
}

// TestFilterAddWireErrors performs negative tests against wire encode and decode
// of MsgFilterAdd to confirm error paths work correctly.
func TestFilterAddWireErrors(t *testing.T) {
	pver := ProtocolVersion

	baseData := []byte{0x01, 0x02, 0x03, 0x04}
	baseFilterAdd := NewMsgFilterAdd(baseData)
	baseFilterAddEncoded := append([]byte{0x04}, baseData...)

	tests := []struct {
		max      int
		writeErr error
		readErr  error
	}{
		{0, io.ErrShortWrite, io.EOF}, // Force error in data size.
		{1, io.ErrShortWrite, io.EOF}, // Force error in data.
	}

	for i, test := range tests {
		w := newFixedWriter(test.max)
		err := baseFilterAdd.LatEncode(w, pver, BaseEncoding)
		require.ErrorIs(t, err, test.writeErr, "encode test #%d", i)

		var msg MsgFilterAdd
		r := newFixedReader(test.max, baseFilterAddEncoded)
		err = msg.LatDecode(r, pver, BaseEncoding)
		require.ErrorIs(t, err, test.readErr, "decode test #%d", i)
	}
}
