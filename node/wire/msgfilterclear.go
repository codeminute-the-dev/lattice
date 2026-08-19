// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"io"
)

// MsgFilterClear implements the Message interface and represents a
// filterclear message which is used to reset a Bloom filter.
//
// This message has no payload.
type MsgFilterClear struct{}

// LatDecode decodes r using the wire protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgFilterClear) LatDecode(r io.Reader, pver uint32, enc MessageEncoding) error {
	return nil
}

// LatEncode encodes the receiver to w using the wire protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgFilterClear) LatEncode(w io.Writer, pver uint32, enc MessageEncoding) error {
	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgFilterClear) Command() string {
	return CmdFilterClear
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgFilterClear) MaxPayloadLength(pver uint32) uint32 {
	return 0
}

// NewMsgFilterClear returns a new filterclear message that conforms to the Message
// interface.  See MsgFilterClear for details.
func NewMsgFilterClear() *MsgFilterClear {
	return &MsgFilterClear{}
}
