// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"io"
)

// MsgPong implements the Message interface and represents a pong
// message which is used primarily to confirm that a connection is still valid
// in response to a ping message (MsgPing).
type MsgPong struct {
	// Unique value associated with message that is used to identify
	// specific ping message.
	Nonce uint64
}

// LatDecode decodes r using the wire protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgPong) LatDecode(r io.Reader, pver uint32, enc MessageEncoding) error {
	nonce, err := binarySerializer.Uint64(r, littleEndian)
	if err != nil {
		return err
	}
	msg.Nonce = nonce
	return nil
}

// LatEncode encodes the receiver to w using the wire protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgPong) LatEncode(w io.Writer, pver uint32, enc MessageEncoding) error {
	return binarySerializer.PutUint64(w, littleEndian, msg.Nonce)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgPong) Command() string {
	return CmdPong
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgPong) MaxPayloadLength(pver uint32) uint32 {
	// Nonce 8 bytes.
	return 8
}

// NewMsgPong returns a new pong message that conforms to the Message
// interface.  See MsgPong for details.
func NewMsgPong(nonce uint64) *MsgPong {
	return &MsgPong{
		Nonce: nonce,
	}
}
