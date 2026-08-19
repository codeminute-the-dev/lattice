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

// TestNetAddress tests the NetAddress API.
func TestNetAddress(t *testing.T) {
	ip := net.ParseIP("127.0.0.1")
	port := 8333

	na := NewNetAddress(&net.TCPAddr{IP: ip, Port: port}, 0)

	require.True(t, na.IP.Equal(ip))
	require.Equal(t, uint16(port), na.Port)
	require.Equal(t, ServiceFlag(0), na.Services)
	require.False(t, na.HasService(SFNodeNetwork))

	na.AddService(SFNodeNetwork)
	require.Equal(t, SFNodeNetwork, na.Services)
	require.True(t, na.HasService(SFNodeNetwork))

	// Payload: timestamp (4) + services (8) + ip (16) + port (2) = 30 bytes.
	require.Equal(t, uint32(30), maxNetAddressPayload(ProtocolVersion))
}

// TestNetAddressWire tests the NetAddress wire encode and decode for various
// timestamp flag combinations.
func TestNetAddressWire(t *testing.T) {
	// baseNetAddr is used in the various tests as a baseline NetAddress.
	baseNetAddr := NetAddress{
		Timestamp: time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST
		Services:  SFNodeNetwork,
		IP:        net.ParseIP("127.0.0.1"),
		Port:      8333,
	}

	// baseNetAddrNoTS is baseNetAddr with a zero value for the timestamp.
	baseNetAddrNoTS := baseNetAddr
	baseNetAddrNoTS.Timestamp = time.Time{}

	// baseNetAddrEncoded is the wire encoded bytes of baseNetAddr.
	baseNetAddrEncoded := []byte{
		0x29, 0xab, 0x5f, 0x49, // Timestamp
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SFNodeNetwork
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xff, 0xff, 0x7f, 0x00, 0x00, 0x01, // IP 127.0.0.1
		0x20, 0x8d, // Port 8333 in big-endian
	}

	// baseNetAddrNoTSEncoded is the wire encoded bytes of baseNetAddrNoTS.
	baseNetAddrNoTSEncoded := []byte{
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SFNodeNetwork
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xff, 0xff, 0x7f, 0x00, 0x00, 0x01, // IP 127.0.0.1
		0x20, 0x8d, // Port 8333 in big-endian
	}

	tests := []struct {
		in   NetAddress
		out  NetAddress
		ts   bool // Include timestamp?
		buf  []byte
		pver uint32
	}{
		// Without ts flag (timestamp not included in wire format).
		{baseNetAddr, baseNetAddrNoTS, false, baseNetAddrNoTSEncoded, ProtocolVersion},
		// With ts flag (timestamp included in wire format).
		{baseNetAddr, baseNetAddr, true, baseNetAddrEncoded, ProtocolVersion},
	}

	for i, test := range tests {
		var buf bytes.Buffer
		err := writeNetAddress(&buf, test.pver, &test.in, test.ts)
		require.NoError(t, err, "test #%d", i)
		require.Equal(t, test.buf, buf.Bytes(), "test #%d", i)

		var na NetAddress
		rbuf := bytes.NewReader(test.buf)
		err = readNetAddress(rbuf, test.pver, &na, test.ts)
		require.NoError(t, err, "test #%d", i)
		require.Equal(t, test.out, na, "test #%d", i)
	}
}

// TestNetAddressWireErrors performs negative tests against wire encode and
// decode NetAddress to confirm error paths work correctly.
func TestNetAddressWireErrors(t *testing.T) {
	pver := ProtocolVersion

	// baseNetAddr is used in the various tests as a baseline NetAddress.
	baseNetAddr := NetAddress{
		Timestamp: time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST
		Services:  SFNodeNetwork,
		IP:        net.ParseIP("127.0.0.1"),
		Port:      8333,
	}

	tests := []struct {
		in       *NetAddress
		ts       bool
		max      int
		writeErr error
		readErr  error
	}{
		// With timestamp - force errors on timestamp, services, ip, port.
		{&baseNetAddr, true, 0, io.ErrShortWrite, io.EOF},
		{&baseNetAddr, true, 4, io.ErrShortWrite, io.EOF},
		{&baseNetAddr, true, 12, io.ErrShortWrite, io.EOF},
		{&baseNetAddr, true, 28, io.ErrShortWrite, io.EOF},
		// Without timestamp - force errors on services, ip, port.
		{&baseNetAddr, false, 0, io.ErrShortWrite, io.EOF},
		{&baseNetAddr, false, 8, io.ErrShortWrite, io.EOF},
		{&baseNetAddr, false, 24, io.ErrShortWrite, io.EOF},
	}

	for i, test := range tests {
		w := newFixedWriter(test.max)
		err := writeNetAddress(w, pver, test.in, test.ts)
		require.ErrorIs(t, err, test.writeErr, "write test #%d", i)

		var na NetAddress
		r := newFixedReader(test.max, []byte{})
		err = readNetAddress(r, pver, &na, test.ts)
		require.ErrorIs(t, err, test.readErr, "read test #%d", i)
	}
}
