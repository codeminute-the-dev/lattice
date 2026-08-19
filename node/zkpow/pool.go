//go:build zkpow

// Copyright (c) 2026 The Lattice contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package zkpow

/*
#include "../../zk-pow/bindings/go/zk_pow_ffi.h"
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/codeminute-the-dev/lattice/node/wire"
)

// The pool side of the FFI: what a stratum server needs and a node does not.
//
// A miner does not send back a certificate. It sends a PlainProof — the raw
// search result, blake3 and Merkle openings, no plonky2 — because producing the
// ZK certificate is expensive and belongs on the pool, once, for the one share
// that turns out to be a block. So the pool does two things a node never does:
// it checks a proof against a share target far easier than consensus requires,
// and it later promotes exactly one of those proofs into a real certificate.

// MaxPlainProofSize bounds a submitted share before it reaches the FFI.
//
// verify_plain_proof_ffi does not bound its input and PlainProof::deserialize
// has no internal limit, so an unbounded submission is unbounded work inside
// the call. Anything this size is already far past a legitimate share.
const MaxPlainProofSize = 8 << 20 // 8 MiB

// DefaultMiningConfig returns the canonical serialized mining configuration.
// e == 0 selects a standard (dense) job.
//
// The configuration is a constant, not derived from the block, which is why a
// stratum job does not carry it: both sides already agree on it.
func DefaultMiningConfig(e, topK uint32) ([]byte, error) {
	cfg, err := defaultMiningConfig(e, topK)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(cfg))
	copy(out, cfg[:])
	return out, nil
}

// VerifyPlainProof checks a submitted share against the header.
//
// nbitsOverride is the pool's share target; pass 0 to check against the
// header's own difficulty, which is the question "is this share also a block?".
// The header's nbits is never modified either way: the proof commitment is
// derived from the header including its nbits, so it has to stay exactly what
// the miner mined.
func VerifyPlainProof(header *wire.BlockHeader, plainProof []byte,
	certVersion wire.CertificateVersion, nbitsOverride uint32) error {

	if len(plainProof) == 0 {
		return fmt.Errorf("empty plain proof")
	}
	if len(plainProof) > MaxPlainProofSize {
		return fmt.Errorf("plain proof too large: %d bytes (max %d)",
			len(plainProof), MaxPlainProofSize)
	}

	cHeader := blockHeaderToC(header)

	var pinner runtime.Pinner
	pinner.Pin(&plainProof[0])
	defer pinner.Unpin()

	var errorBuf [C.ERROR_MSG_MAX_SIZE]C.char
	result := C.verify_plain_proof_ffi(
		&cHeader,
		(*C.uint8_t)(unsafe.Pointer(&plainProof[0])),
		C.uintptr_t(len(plainProof)),
		C.uint32_t(certVersion),
		C.uint32_t(nbitsOverride),
		&errorBuf[0],
	)
	msg := C.GoString(&errorBuf[0])

	switch result {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("share rejected: %s", msg)
	default:
		return fmt.Errorf("share verification error (code %d): %s", result, msg)
	}
}

// ProvePlainProof turns a share that met the network target into a full ZK
// certificate ready to go in a block.
//
// This is the expensive one — it runs plonky2 and needs the circuit cache — so
// it is called once per block found, never per share.
func ProvePlainProof(header *wire.BlockHeader, plainProof []byte,
	certVersion wire.CertificateVersion) (wire.BlockCertificate, error) {

	if len(plainProof) == 0 {
		return nil, fmt.Errorf("empty plain proof")
	}
	if len(plainProof) > MaxPlainProofSize {
		return nil, fmt.Errorf("plain proof too large: %d bytes (max %d)",
			len(plainProof), MaxPlainProofSize)
	}

	cHeader := blockHeaderToC(header)

	var pinner runtime.Pinner
	pinner.Pin(&plainProof[0])
	defer pinner.Unpin()

	// The proof blob buffer has to be Go-allocated and pinned: the Rust side
	// writes into it rather than returning ownership.
	proofBlob := make([]byte, C.MAX_ZK_PROOF_SIZE)
	pinner.Pin(&proofBlob[0])

	var out C.CZKProof
	out.proof_blob = (*C.uint8_t)(unsafe.Pointer(&proofBlob[0]))
	out.proof_blob_len = C.uintptr_t(len(proofBlob))

	var errorBuf [C.ERROR_MSG_MAX_SIZE]C.char
	result := C.prove_plain_proof_ffi(
		&cHeader,
		(*C.uint8_t)(unsafe.Pointer(&plainProof[0])),
		C.uintptr_t(len(plainProof)),
		C.uint32_t(certVersion),
		&out,
		&errorBuf[0],
	)
	if result != 0 {
		return nil, fmt.Errorf("proving failed (code %d): %s",
			result, C.GoString(&errorBuf[0]))
	}

	publicDataLen := int(out.public_data_len)
	if publicDataLen == 0 || publicDataLen > wire.PublicDataMaxSizeV2 {
		return nil, fmt.Errorf("unexpected public_data_len %d (max %d)",
			publicDataLen, wire.PublicDataMaxSizeV2)
	}
	publicData := C.GoBytes(unsafe.Pointer(&out.public_data[0]), C.int(publicDataLen))
	proofData := C.GoBytes(unsafe.Pointer(out.proof_blob), C.int(out.proof_blob_len))

	return newCertificate(header, certVersion, publicData, proofData)
}
