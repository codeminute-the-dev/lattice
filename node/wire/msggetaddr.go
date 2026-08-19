// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"io"
)

// MsgGetAddr implements the Message interface and represents a
// getaddr message.  It is used to request a list of known active peers on the
// network from a peer to help identify potential nodes.  The list is returned
// via one or more addr messages (MsgAddr).
//
// This message has no payload.
type MsgGetAddr struct{}

// LatDecode decodes r using the wire protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgGetAddr) LatDecode(r io.Reader, pver uint32, enc MessageEncoding) error {
	return nil
}

// LatEncode encodes the receiver to w using the wire protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgGetAddr) LatEncode(w io.Writer, pver uint32, enc MessageEncoding) error {
	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgGetAddr) Command() string {
	return CmdGetAddr
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgGetAddr) MaxPayloadLength(pver uint32) uint32 {
	return 0
}

// NewMsgGetAddr returns a new getaddr message that conforms to the
// Message interface.  See MsgGetAddr for details.
func NewMsgGetAddr() *MsgGetAddr {
	return &MsgGetAddr{}
}
