// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"io"
)

// MsgPing implements the Message interface and represents a ping
// message used to confirm that a connection is still valid. It contains an
// identifier which can be returned in the pong message to determine network
// timing.
//
// The payload for this message just consists of a nonce used for identifying
// it later.
type MsgPing struct {
	// Unique value associated with message that is used to identify
	// specific ping message.
	Nonce uint64
}

// LatDecode decodes r using the wire protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgPing) LatDecode(r io.Reader, pver uint32, enc MessageEncoding) error {
	nonce, err := binarySerializer.Uint64(r, littleEndian)
	if err != nil {
		return err
	}
	msg.Nonce = nonce
	return nil
}

// LatEncode encodes the receiver to w using the wire protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgPing) LatEncode(w io.Writer, pver uint32, enc MessageEncoding) error {
	return binarySerializer.PutUint64(w, littleEndian, msg.Nonce)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgPing) Command() string {
	return CmdPing
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgPing) MaxPayloadLength(pver uint32) uint32 {
	// Nonce 8 bytes.
	return 8
}

// NewMsgPing returns a new ping message that conforms to the Message
// interface.  See MsgPing for details.
func NewMsgPing(nonce uint64) *MsgPing {
	return &MsgPing{
		Nonce: nonce,
	}
}
