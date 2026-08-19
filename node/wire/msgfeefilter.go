// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"io"
)

// MsgFeeFilter implements the Message interface and represents a
// feefilter message.  It is used to request the receiving peer does not
// announce any transactions below the specified minimum fee rate.
type MsgFeeFilter struct {
	MinFee int64
}

// LatDecode decodes r using the wire protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgFeeFilter) LatDecode(r io.Reader, pver uint32, enc MessageEncoding) error {
	return readElement(r, &msg.MinFee)
}

// LatEncode encodes the receiver to w using the wire protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgFeeFilter) LatEncode(w io.Writer, pver uint32, enc MessageEncoding) error {
	return writeElement(w, msg.MinFee)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgFeeFilter) Command() string {
	return CmdFeeFilter
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgFeeFilter) MaxPayloadLength(pver uint32) uint32 {
	return 8
}

// NewMsgFeeFilter returns a new feefilter message that conforms to
// the Message interface.  See MsgFeeFilter for details.
func NewMsgFeeFilter(minfee int64) *MsgFeeFilter {
	return &MsgFeeFilter{
		MinFee: minfee,
	}
}
