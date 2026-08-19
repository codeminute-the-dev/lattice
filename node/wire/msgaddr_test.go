// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAddr tests the MsgAddr API.
func TestAddr(t *testing.T) {
	pver := ProtocolVersion

	msg := NewMsgAddr()
	require.Equal(t, "addr", msg.Command())
	// Max payload: Num addresses (varInt) + max allowed addresses.
	require.Equal(t, uint32(30009), msg.MaxPayloadLength(pver))

	// Ensure NetAddresses are added properly.
	tcpAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8333}
	na := NewNetAddress(tcpAddr, SFNodeNetwork)
	err := msg.AddAddress(na)
	require.NoError(t, err)
	require.Equal(t, na, msg.AddrList[0])

	// Ensure the address list is cleared properly.
	msg.ClearAddresses()
	require.Empty(t, msg.AddrList)

	// Ensure adding more than the max allowed addresses per message returns error.
	for i := 0; i < MaxAddrPerMsg+1; i++ {
		err = msg.AddAddress(na)
	}
	require.Error(t, err)

	err = msg.AddAddresses(na)
	require.Error(t, err)
}

// TestAddrWire tests the MsgAddr wire encode and decode.
func TestAddrWire(t *testing.T) {
	// A couple of NetAddresses to use for testing.
	na := &NetAddress{
		Timestamp: time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST
		Services:  SFNodeNetwork,
		IP:        net.ParseIP("127.0.0.1"),
		Port:      8333,
	}
	na2 := &NetAddress{
		Timestamp: time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST
		Services:  SFNodeNetwork,
		IP:        net.ParseIP("192.168.0.1"),
		Port:      8334,
	}

	// Empty address message.
	noAddr := NewMsgAddr()
	noAddrEncoded := []byte{
		0x00, // Varint for number of addresses
	}

	// Address message with multiple addresses.
	multiAddr := NewMsgAddr()
	multiAddr.AddAddresses(na, na2)
	multiAddrEncoded := []byte{
		0x02,                   // Varint for number of addresses
		0x29, 0xab, 0x5f, 0x49, // Timestamp
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SFNodeNetwork
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xff, 0xff, 0x7f, 0x00, 0x00, 0x01, // IP 127.0.0.1
		0x20, 0x8d, // Port 8333 in big-endian
		0x29, 0xab, 0x5f, 0x49, // Timestamp
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SFNodeNetwork
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xff, 0xff, 0xc0, 0xa8, 0x00, 0x01, // IP 192.168.0.1
		0x20, 0x8e, // Port 8334 in big-endian
	}

	tests := []struct {
		in   *MsgAddr
		out  *MsgAddr
		buf  []byte
		pver uint32
	}{
		{noAddr, noAddr, noAddrEncoded, ProtocolVersion},          // Latest protocol version with no addresses.
		{multiAddr, multiAddr, multiAddrEncoded, ProtocolVersion}, // Latest protocol version with multiple addresses.
		{noAddr, noAddr, noAddrEncoded, ProtocolVersion - 1},      // Protocol version ProtocolVersion with no addresses.
	}

	for i, test := range tests {
		var buf bytes.Buffer
		err := test.in.LatEncode(&buf, test.pver, BaseEncoding)
		require.NoError(t, err, "test #%d", i)
		require.Equal(t, test.buf, buf.Bytes(), "test #%d", i)

		var msg MsgAddr
		rbuf := bytes.NewReader(test.buf)
		err = msg.LatDecode(rbuf, test.pver, BaseEncoding)
		require.NoError(t, err, "test #%d", i)
		require.Equal(t, test.out, &msg, "test #%d", i)
	}
}

// TestAddrWireErrors performs negative tests against wire encode and decode
// of MsgAddr to confirm error paths work correctly.
func TestAddrWireErrors(t *testing.T) {
	pver := ProtocolVersion

	// A couple of NetAddresses to use for testing.
	na := &NetAddress{
		Timestamp: time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST
		Services:  SFNodeNetwork,
		IP:        net.ParseIP("127.0.0.1"),
		Port:      8333,
	}
	na2 := &NetAddress{
		Timestamp: time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST
		Services:  SFNodeNetwork,
		IP:        net.ParseIP("192.168.0.1"),
		Port:      8334,
	}

	// Address message with multiple addresses.
	baseAddr := NewMsgAddr()
	baseAddr.AddAddresses(na, na2)
	baseAddrEncoded := []byte{
		0x02,                   // Varint for number of addresses
		0x29, 0xab, 0x5f, 0x49, // Timestamp
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SFNodeNetwork
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xff, 0xff, 0x7f, 0x00, 0x00, 0x01, // IP 127.0.0.1
		0x20, 0x8d, // Port 8333 in big-endian
		0x29, 0xab, 0x5f, 0x49, // Timestamp
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SFNodeNetwork
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xff, 0xff, 0xc0, 0xa8, 0x00, 0x01, // IP 192.168.0.1
		0x20, 0x8e, // Port 8334 in big-endian
	}

	// Message that forces an error by having more than the max allowed addresses.
	maxAddr := NewMsgAddr()
	for i := 0; i < MaxAddrPerMsg; i++ {
		maxAddr.AddAddress(na)
	}
	maxAddr.AddrList = append(maxAddr.AddrList, na)
	maxAddrEncoded := []byte{0xfd, 0x03, 0xe9} // Varint for number of addresses (1001)

	tests := []struct {
		in       *MsgAddr
		buf      []byte
		pver     uint32
		max      int
		writeErr error
		readErr  error
	}{
		// Force error in addresses count.
		{baseAddr, baseAddrEncoded, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error in address list.
		{baseAddr, baseAddrEncoded, pver, 1, io.ErrShortWrite, io.EOF},
		// Force error with greater than max inventory vectors.
		{maxAddr, maxAddrEncoded, pver, 3, &MessageError{}, &MessageError{}},
		{maxAddr, maxAddrEncoded, pver - 1, 3, &MessageError{}, &MessageError{}},
	}

	for i, test := range tests {
		w := newFixedWriter(test.max)
		err := test.in.LatEncode(w, test.pver, BaseEncoding)
		require.IsType(t, test.writeErr, err, "encode test #%d", i)
		if _, ok := err.(*MessageError); !ok {
			require.ErrorIs(t, err, test.writeErr, "encode test #%d", i)
		}

		var msg MsgAddr
		r := newFixedReader(test.max, test.buf)
		err = msg.LatDecode(r, test.pver, BaseEncoding)
		require.IsType(t, test.readErr, err, "decode test #%d", i)
		if _, ok := err.(*MessageError); !ok {
			require.ErrorIs(t, err, test.readErr, "decode test #%d", i)
		}
	}
}
