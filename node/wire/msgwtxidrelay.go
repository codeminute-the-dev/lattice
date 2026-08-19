// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"io"
)

// MsgWTxIdRelay defines a wtxidrelay message which is used for a peer
// to signal support for relaying witness transaction id (BIP141). It
// implements the Message interface.
//
// This message has no payload.
type MsgWTxIdRelay struct{}

// LatDecode decodes r using the wire protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgWTxIdRelay) LatDecode(r io.Reader, pver uint32, enc MessageEncoding) error {
	return nil
}

// LatEncode encodes the receiver to w using the wire protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgWTxIdRelay) LatEncode(w io.Writer, pver uint32, enc MessageEncoding) error {
	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgWTxIdRelay) Command() string {
	return CmdWTxIdRelay
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgWTxIdRelay) MaxPayloadLength(pver uint32) uint32 {
	return 0
}

// NewMsgWTxIdRelay returns a new wtxidrelay message that conforms
// to the Message interface.
func NewMsgWTxIdRelay() *MsgWTxIdRelay {
	return &MsgWTxIdRelay{}
}
