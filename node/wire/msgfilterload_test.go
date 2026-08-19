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

// TestFilterLoadLatest tests the MsgFilterLoad API against the latest protocol version.
func TestFilterLoadLatest(t *testing.T) {
	pver := ProtocolVersion
	enc := BaseEncoding

	data := []byte{0x01, 0x02}
	msg := NewMsgFilterLoad(data, 10, 0, 0)

	require.Equal(t, "filterload", msg.Command())
	require.Equal(t, uint32(36012), msg.MaxPayloadLength(pver))

	// Test encode with latest protocol version.
	var buf bytes.Buffer
	err := msg.LatEncode(&buf, pver, enc)
	require.NoError(t, err)

	// Test decode with latest protocol version.
	readmsg := MsgFilterLoad{}
	err = readmsg.LatDecode(&buf, pver, enc)
	require.NoError(t, err)

	require.Equal(t, msg.Filter, readmsg.Filter)
}

// TestFilterLoadMaxFilterSize tests the MsgFilterLoad API maximum filter size.
func TestFilterLoadMaxFilterSize(t *testing.T) {
	data := bytes.Repeat([]byte{0xff}, 36001)
	msg := NewMsgFilterLoad(data, 10, 0, 0)

	// Encode with latest protocol version should fail.
	var buf bytes.Buffer
	err := msg.LatEncode(&buf, ProtocolVersion, BaseEncoding)
	require.Error(t, err)

	// Decode with latest protocol version should fail.
	readbuf := bytes.NewReader(data)
	err = msg.LatDecode(readbuf, ProtocolVersion, BaseEncoding)
	require.Error(t, err)
}

// TestFilterLoadMaxHashFuncsSize tests the MsgFilterLoad API maximum hash functions.
func TestFilterLoadMaxHashFuncsSize(t *testing.T) {
	data := bytes.Repeat([]byte{0xff}, 10)
	msg := NewMsgFilterLoad(data, 61, 0, 0)

	// Encode with latest protocol version should fail.
	var buf bytes.Buffer
	err := msg.LatEncode(&buf, ProtocolVersion, BaseEncoding)
	require.Error(t, err)

	newBuf := []byte{
		0x0a,                                                       // filter size
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, // filter
		0x3d, 0x00, 0x00, 0x00, // max hash funcs
		0x00, 0x00, 0x00, 0x00, // tweak
		0x00, // update Type
	}
	// Decode with latest protocol version should fail.
	readbuf := bytes.NewReader(newBuf)
	err = msg.LatDecode(readbuf, ProtocolVersion, BaseEncoding)
	require.Error(t, err)
}

// TestFilterLoadWireErrors performs negative tests against wire encode and decode
// of MsgFilterLoad to confirm error paths work correctly.
func TestFilterLoadWireErrors(t *testing.T) {
	pver := ProtocolVersion

	baseFilter := []byte{0x01, 0x02, 0x03, 0x04}
	baseFilterLoad := NewMsgFilterLoad(baseFilter, 10, 0, BloomUpdateNone)
	baseFilterLoadEncoded := append([]byte{0x04}, baseFilter...)
	baseFilterLoadEncoded = append(baseFilterLoadEncoded,
		0x00, 0x00, 0x00, 0x0a, // HashFuncs
		0x00, 0x00, 0x00, 0x00, // Tweak
		0x00) // Flags

	tests := []struct {
		max      int
		writeErr error
		readErr  error
	}{
		{0, io.ErrShortWrite, io.EOF},  // Force error in filter size.
		{1, io.ErrShortWrite, io.EOF},  // Force error in filter.
		{5, io.ErrShortWrite, io.EOF},  // Force error in hash funcs.
		{9, io.ErrShortWrite, io.EOF},  // Force error in tweak.
		{13, io.ErrShortWrite, io.EOF}, // Force error in flags.
	}

	for i, test := range tests {
		w := newFixedWriter(test.max)
		err := baseFilterLoad.LatEncode(w, pver, BaseEncoding)
		require.ErrorIs(t, err, test.writeErr, "encode test #%d", i)

		var msg MsgFilterLoad
		r := newFixedReader(test.max, baseFilterLoadEncoded)
		err = msg.LatDecode(r, pver, BaseEncoding)
		require.ErrorIs(t, err, test.readErr, "decode test #%d", i)
	}
}
