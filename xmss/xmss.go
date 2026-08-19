//go:build xmss

// Package xmss provides Go bindings for the XMSS post-quantum signature library.
package xmss

/*
#cgo CFLAGS: -I${SRCDIR}
#cgo LDFLAGS: ${SRCDIR}/libxmss.a -lstdc++

#include "xmss.h"
*/
import "C"
import (
	"errors"
	"unsafe"
)

const (
	PrivateSeedLen = C.PRIVATE_SEED_LEN // 64 bytes
	PublicSeedLen  = C.PUBLIC_SEED_LEN  // 32 bytes
	PKLen          = C.PK_LEN           // 64 bytes
	SKLen          = C.SK_LEN           // 128 bytes
	MsgLen         = C.MSG_LEN          // 32 bytes
	SignatureLen   = C.SIGNATURE_LEN    // 2340 bytes
	MaxSigns       = C.MAX_SIGNS        // 32
)

// Keygen generates an XMSS key pair from seeds.
// privateSeed must be 64 bytes, publicSeed must be 32 bytes.
// Caller should clear sk after use for security.
func Keygen(privateSeed [PrivateSeedLen]byte, publicSeed [PublicSeedLen]byte) (pk [PKLen]byte, sk [SKLen]byte, err error) {
	ret := C.xmss_keygen(
		(*C.uint8_t)(unsafe.Pointer(&privateSeed[0])),
		(*C.uint8_t)(unsafe.Pointer(&publicSeed[0])),
		(*C.uint8_t)(unsafe.Pointer(&pk[0])),
		(*C.uint8_t)(unsafe.Pointer(&sk[0])),
	)

	if ret != 0 {
		return pk, sk, errors.New("xmss_keygen failed")
	}

	return pk, sk, nil
}

// Sign signs a message using the secret key and a unique message ID.
// msgUID must be < XMSS_MaxSigns (32). Each msgUID can only be used once.
// Publishing signatures of two different messages that use the same msgUID
// enables attackers to sign unintended messages in the name of the private_seed owner.
func Sign(msgUID uint32, sk [SKLen]byte, msg [MsgLen]byte) ([SignatureLen]byte, error) {
	var sig [SignatureLen]byte

	ret := C.xmss_sign(
		C.uint(msgUID),
		(*C.uint8_t)(unsafe.Pointer(&sk[0])),
		(*C.uint8_t)(unsafe.Pointer(&msg[0])),
		(*C.uint8_t)(unsafe.Pointer(&sig[0])),
	)

	if ret != 0 {
		return sig, errors.New("xmss_sign failed")
	}

	return sig, nil
}

// Verify verifies a signature against a message and public key.
func Verify(pk [PKLen]byte, msg [MsgLen]byte, sig [SignatureLen]byte) bool {
	ret := C.xmss_verify(
		(*C.uint8_t)(unsafe.Pointer(&pk[0])),
		(*C.uint8_t)(unsafe.Pointer(&msg[0])),
		(*C.uint8_t)(unsafe.Pointer(&sig[0])),
	)
	return ret == 0
}
