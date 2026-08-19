// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"io"
)

// MsgMemPool implements the Message interface and represents a mempool
// message.  It is used to request a list of transactions still in the active
// memory pool of a relay.
//
// This message has no payload.
type MsgMemPool struct{}

// LatDecode decodes r using the wire protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgMemPool) LatDecode(r io.Reader, pver uint32, enc MessageEncoding) error {
	return nil
}

// LatEncode encodes the receiver to w using the wire protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgMemPool) LatEncode(w io.Writer, pver uint32, enc MessageEncoding) error {
	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgMemPool) Command() string {
	return CmdMemPool
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgMemPool) MaxPayloadLength(pver uint32) uint32 {
	return 0
}

// NewMsgMemPool returns a new pong message that conforms to the Message
// interface.  See MsgPong for details.
func NewMsgMemPool() *MsgMemPool {
	return &MsgMemPool{}
}
