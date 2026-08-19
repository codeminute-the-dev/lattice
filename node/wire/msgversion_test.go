// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestVersion tests the MsgVersion API.
func TestVersion(t *testing.T) {
	pver := ProtocolVersion

	lastBlock := int32(234234)
	tcpAddrMe := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8333}
	me := NewNetAddress(tcpAddrMe, SFNodeNetwork)
	tcpAddrYou := &net.TCPAddr{IP: net.ParseIP("192.168.0.1"), Port: 8333}
	you := NewNetAddress(tcpAddrYou, SFNodeNetwork)
	nonce, err := RandomUint64()
	require.NoError(t, err)

	msg := NewMsgVersion(me, you, nonce, lastBlock)
	require.Equal(t, int32(pver), msg.ProtocolVersion)
	require.Equal(t, me, &msg.AddrMe)
	require.Equal(t, you, &msg.AddrYou)
	require.Equal(t, nonce, msg.Nonce)
	require.Equal(t, DefaultUserAgent, msg.UserAgent)
	require.Equal(t, lastBlock, msg.LastBlock)
	require.False(t, msg.DisableRelayTx)

	msg.AddUserAgent("myclient", "1.2.3", "optional", "comments")
	customUserAgent := DefaultUserAgent + "myclient:1.2.3(optional; comments)/"
	require.Equal(t, customUserAgent, msg.UserAgent)

	msg.AddUserAgent("mygui", "3.4.5")
	customUserAgent += "mygui:3.4.5/"
	require.Equal(t, customUserAgent, msg.UserAgent)

	// Test user agent length limit.
	err = msg.AddUserAgent(strings.Repeat("t", MaxUserAgentLen-len(customUserAgent)-2+1), "")
	require.IsType(t, &MessageError{}, err)

	// Version message should not have any services set by default.
	require.Equal(t, ServiceFlag(0), msg.Services)
	require.False(t, msg.HasService(SFNodeNetwork))

	require.Equal(t, "version", msg.Command())
	// Protocol version 4 bytes + services 8 bytes + timestamp 8 bytes +
	// remote and local net addresses + nonce 8 bytes + length of user agent
	// (varInt) + max allowed user agent length + last block 4 bytes +
	// relay transactions flag 1 byte.
	// = 4 + 8 + 8 + 30 + 8 + 1 + 256 + 4 + 1 = 358
	require.Equal(t, uint32(358), msg.MaxPayloadLength(pver))

	msg.AddService(SFNodeNetwork)
	require.Equal(t, SFNodeNetwork, msg.Services)
	require.True(t, msg.HasService(SFNodeNetwork))
}

// TestVersionWire tests the MsgVersion wire encode and decode.
func TestVersionWire(t *testing.T) {
	// verRelayTxFalse is a version message with transaction relay disabled.
	baseVersionCopy := *baseVersion
	verRelayTxFalse := &baseVersionCopy
	verRelayTxFalse.DisableRelayTx = true
	verRelayTxFalseEncoded := make([]byte, len(baseVersionEncoded))
	copy(verRelayTxFalseEncoded, baseVersionEncoded)
	verRelayTxFalseEncoded[len(verRelayTxFalseEncoded)-1] = 0

	tests := []struct {
		in  *MsgVersion
		out *MsgVersion
		buf []byte
	}{
		// Basic version message (relay tx enabled by default).
		{baseVersion, baseVersion, baseVersionEncoded},
		// Version message with transaction relay disabled.
		{verRelayTxFalse, verRelayTxFalse, verRelayTxFalseEncoded},
	}

	for i, test := range tests {
		var buf bytes.Buffer
		err := test.in.LatEncode(&buf, ProtocolVersion, BaseEncoding)
		require.NoError(t, err, "test #%d", i)
		require.Equal(t, test.buf, buf.Bytes(), "test #%d", i)

		var msg MsgVersion
		rbuf := bytes.NewBuffer(test.buf)
		err = msg.LatDecode(rbuf, ProtocolVersion, BaseEncoding)
		require.NoError(t, err, "test #%d", i)
		require.Equal(t, test.out, &msg, "test #%d", i)
	}
}

// TestVersionWireErrors performs negative tests against wire encode and
// decode of MsgVersion to confirm error paths work correctly.
func TestVersionWireErrors(t *testing.T) {
	pver := ProtocolVersion
	enc := BaseEncoding

	// Ensure calling MsgVersion.LatDecode with a non *bytes.Buffer returns error.
	fr := newFixedReader(0, []byte{})
	err := baseVersion.LatDecode(fr, pver, enc)
	require.Error(t, err)

	// Copy the base version and change the user agent to exceed max limits.
	bvc := *baseVersion
	exceedUAVer := &bvc
	newUA := "/" + strings.Repeat("t", MaxUserAgentLen-8+1) + ":0.0.1/"
	exceedUAVer.UserAgent = newUA

	// Encode the new UA length as a varint.
	var newUAVarIntBuf bytes.Buffer
	err = WriteVarInt(&newUAVarIntBuf, pver, uint64(len(newUA)))
	require.NoError(t, err)

	newLen := len(baseVersionEncoded) - len(baseVersion.UserAgent)
	newLen = newLen + len(newUAVarIntBuf.Bytes()) - 1 + len(newUA)
	exceedUAVerEncoded := make([]byte, newLen)
	copy(exceedUAVerEncoded, baseVersionEncoded[0:80])
	copy(exceedUAVerEncoded[80:], newUAVarIntBuf.Bytes())
	copy(exceedUAVerEncoded[83:], []byte(newUA))
	copy(exceedUAVerEncoded[83+len(newUA):], baseVersionEncoded[97:100])

	tests := []struct {
		in       *MsgVersion
		buf      []byte
		max      int
		writeErr error
		readErr  error
	}{
		// Force error in protocol version.
		{baseVersion, baseVersionEncoded, 0, io.ErrShortWrite, io.EOF},
		// Force error in services.
		{baseVersion, baseVersionEncoded, 4, io.ErrShortWrite, io.EOF},
		// Force error in timestamp.
		{baseVersion, baseVersionEncoded, 12, io.ErrShortWrite, io.EOF},
		// Force error in remote address.
		{baseVersion, baseVersionEncoded, 20, io.ErrShortWrite, io.EOF},
		// Force error in local address.
		{baseVersion, baseVersionEncoded, 47, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in nonce.
		{baseVersion, baseVersionEncoded, 73, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in user agent length.
		{baseVersion, baseVersionEncoded, 81, io.ErrShortWrite, io.EOF},
		// Force error in user agent.
		{baseVersion, baseVersionEncoded, 82, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in last block.
		{baseVersion, baseVersionEncoded, 98, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in relay tx - no read error since relay is optional.
		{baseVersion, baseVersionEncoded, 101, io.ErrShortWrite, nil},
		// Force error due to user agent too big.
		{exceedUAVer, exceedUAVerEncoded, newLen, &MessageError{}, &MessageError{}},
	}

	for i, test := range tests {
		w := newFixedWriter(test.max)
		err := test.in.LatEncode(w, pver, enc)
		require.IsType(t, test.writeErr, err, "encode test #%d", i)
		if _, ok := err.(*MessageError); !ok && err != nil {
			require.ErrorIs(t, err, test.writeErr, "encode test #%d", i)
		}

		var msg MsgVersion
		buf := bytes.NewBuffer(test.buf[0:test.max])
		err = msg.LatDecode(buf, pver, enc)
		if test.readErr == nil {
			require.NoError(t, err, "decode test #%d", i)
		} else {
			require.IsType(t, test.readErr, err, "decode test #%d", i)
			if _, ok := err.(*MessageError); !ok && err != nil {
				require.ErrorIs(t, err, test.readErr, "decode test #%d", i)
			}
		}
	}
}

// baseVersion is used in the various tests as a baseline MsgVersion.
var baseVersion = &MsgVersion{
	ProtocolVersion: 1,
	Services:        SFNodeNetwork,
	Timestamp:       time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST)
	AddrYou: NetAddress{
		Timestamp: time.Time{}, // Zero value -- no timestamp in version
		Services:  SFNodeNetwork,
		IP:        net.ParseIP("192.168.0.1"),
		Port:      8333,
	},
	AddrMe: NetAddress{
		Timestamp: time.Time{}, // Zero value -- no timestamp in version
		Services:  SFNodeNetwork,
		IP:        net.ParseIP("127.0.0.1"),
		Port:      8333,
	},
	Nonce:     123123, // 0x1e0f3
	UserAgent: "/btcdtest:0.0.1/",
	LastBlock: 234234, // 0x392fa
}

// baseVersionEncoded is the wire encoded bytes for baseVersion.
var baseVersionEncoded = []byte{
	0x01, 0x00, 0x00, 0x00, // Protocol version 1
	0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SFNodeNetwork
	0x29, 0xab, 0x5f, 0x49, 0x00, 0x00, 0x00, 0x00, // 64-bit Timestamp
	// AddrYou -- No timestamp for NetAddress in version message
	0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SFNodeNetwork
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0xff, 0xff, 0xc0, 0xa8, 0x00, 0x01, // IP 192.168.0.1
	0x20, 0x8d, // Port 8333 in big-endian
	// AddrMe -- No timestamp for NetAddress in version message
	0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SFNodeNetwork
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0xff, 0xff, 0x7f, 0x00, 0x00, 0x01, // IP 127.0.0.1
	0x20, 0x8d, // Port 8333 in big-endian
	0xf3, 0xe0, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, // Nonce
	0x10, // Varint for user agent length
	0x2f, 0x62, 0x74, 0x63, 0x64, 0x74, 0x65, 0x73,
	0x74, 0x3a, 0x30, 0x2e, 0x30, 0x2e, 0x31, 0x2f, // User agent
	0xfa, 0x92, 0x03, 0x00, // Last block
	0x01, // Relay tx
}
