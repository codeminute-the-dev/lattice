//go:build !xmss

package xmss

import "errors"

const (
	PrivateSeedLen = 64
	PublicSeedLen  = 32
	PKLen          = 64
	SKLen          = 128
	MsgLen         = 32
	SignatureLen   = 2340
	MaxSigns       = 32
)

var errNoCgo = errors.New("xmss: build with -tags xmss to enable cryptographic operations")

func Keygen(privateSeed [PrivateSeedLen]byte, publicSeed [PublicSeedLen]byte) (pk [PKLen]byte, sk [SKLen]byte, err error) {
	return pk, sk, errNoCgo
}

func Sign(msgUID uint32, sk [SKLen]byte, msg [MsgLen]byte) ([SignatureLen]byte, error) {
	return [SignatureLen]byte{}, errNoCgo
}

func Verify(pk [PKLen]byte, msg [MsgLen]byte, sig [SignatureLen]byte) bool {
	return false
}
