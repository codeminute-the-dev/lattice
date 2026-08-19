package wire

import (
	"io"
)

// MsgSendAddrV2 defines a sendaddrv2 message which is used for a peer
// to signal support for receiving ADDRV2 messages (BIP155). It implements the
// Message interface.
//
// This message has no payload.
type MsgSendAddrV2 struct{}

// LatDecode decodes r using the wire protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgSendAddrV2) LatDecode(r io.Reader, pver uint32, enc MessageEncoding) error {
	return nil
}

// LatEncode encodes the receiver to w using the wire protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgSendAddrV2) LatEncode(w io.Writer, pver uint32, enc MessageEncoding) error {
	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgSendAddrV2) Command() string {
	return CmdSendAddrV2
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgSendAddrV2) MaxPayloadLength(pver uint32) uint32 {
	return 0
}

// NewMsgSendAddrV2 returns a new sendaddrv2 message that conforms to the
// Message interface.
func NewMsgSendAddrV2() *MsgSendAddrV2 {
	return &MsgSendAddrV2{}
}
