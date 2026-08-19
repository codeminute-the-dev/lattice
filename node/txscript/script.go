// Copyright (c) 2025-2026 The Pearl Research Labs
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	// TaprootAnnexTag is the tag for an annex. This value is used to
	// identify the annex during tapscript spends. If there're at least two
	// elements in the taproot witness stack, and the first byte of the
	// last element matches this tag, then we'll extract this as a distinct
	// item.
	TaprootAnnexTag = 0x50

	// TaprootLeafMask is the mask applied to the control block to extract
	// the leaf version and parity of the y-coordinate of the output key if
	// the taproot script leaf being spent.
	TaprootLeafMask = 0xfe
)

// These are the constants specified for maximums in individual scripts.
const (
	MaxOpsPerScript      = 201 // Max number of non-push operations.
	MaxScriptElementSize = 520 // Max bytes pushable to the stack.
)

// IsSmallInt returns whether or not the opcode is considered a small integer,
// which is an OP_0, or OP_1 through OP_16.
//
// NOTE: This function is only valid for version 0 opcodes.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func IsSmallInt(op byte) bool {
	return op == OP_0 || (op >= OP_1 && op <= OP_16)
}

// IsPayToTaproot returns true if the passed script is a standard
// pay-to-taproot (PTTR) scripts, and false otherwise.
func IsPayToTaproot(script []byte) bool {
	return isWitnessTaprootScript(script)
}

// IsPayToMerkleRoot returns true if the passed script is a standard
// pay-to-merkle-root (P2MR, BIP 360) script, and false otherwise.
func IsPayToMerkleRoot(script []byte) bool {
	return isWitnessMerkleRootScript(script)
}

// IsWitnessProgram returns true if the passed script is a valid witness
// program which is encoded according to the passed witness program version. A
// witness program must be a small integer (from 0-16), followed by 2-40 bytes
// of pushed data.
func IsWitnessProgram(script []byte) bool {
	return isWitnessProgramScript(script)
}

// IsNullData returns true if the passed script is a null data script, false
// otherwise.
func IsNullData(script []byte) bool {
	return isNullDataScript(script)
}

// ExtractWitnessProgramInfo attempts to extract the witness program version,
// as well as the witness program itself from the passed script.
func ExtractWitnessProgramInfo(script []byte) (int, []byte, error) {
	// If at this point, the scripts doesn't resemble a witness program,
	// then we'll exit early as there isn't a valid version or program to
	// extract.
	version, program, valid := extractWitnessProgramInfo(script)
	if !valid {
		return 0, nil, fmt.Errorf("script is not a witness program, " +
			"unable to extract version or witness program")
	}

	return version, program, nil
}

// IsPushOnlyScript returns whether or not the passed script only pushes data
// according to the consensus definition of pushing data.
//
// WARNING: This function always treats the passed script as version 0.  Great
// care must be taken if introducing a new script version because it is used in
// consensus which, unfortunately as of the time of this writing, does not check
// script versions before checking if it is a push only script which means nodes
// on existing rules will treat new version scripts as if they were version 0.
func IsPushOnlyScript(script []byte) bool {
	const scriptVersion = 0
	tokenizer := MakeScriptTokenizer(scriptVersion, script)
	for tokenizer.Next() {
		// All opcodes up to OP_16 are data push instructions.
		// NOTE: This does consider OP_RESERVED to be a data push instruction,
		// but execution of OP_RESERVED will fail anyway and matches the
		// behavior required by consensus.
		if tokenizer.Opcode() > OP_16 {
			return false
		}
	}
	return tokenizer.Err() == nil
}

// DisasmString formats a disassembled script for one line printing.  When the
// script fails to parse, the returned string will contain the disassembled
// script up to the point the failure occurred along with the string '[error]'
// appended.  In addition, the reason the script failed to parse is returned
// if the caller wants more information about the failure.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func DisasmString(script []byte) (string, error) {
	const scriptVersion = 0

	var disbuf strings.Builder
	tokenizer := MakeScriptTokenizer(scriptVersion, script)
	if tokenizer.Next() {
		disasmOpcode(&disbuf, tokenizer.op, tokenizer.Data(), true)
	}
	for tokenizer.Next() {
		disbuf.WriteByte(' ')
		disasmOpcode(&disbuf, tokenizer.op, tokenizer.Data(), true)
	}
	if tokenizer.Err() != nil {
		if tokenizer.ByteIndex() != 0 {
			disbuf.WriteByte(' ')
		}
		disbuf.WriteString("[error]")
	}
	return disbuf.String(), tokenizer.Err()
}

// removeOpcodeRaw will return the script after removing any opcodes that match
// `opcode`. If the opcode does not appear in script, the original script will
// be returned unmodified. Otherwise, a new script will be allocated to contain
// the filtered script. This method assumes that the script parses
// successfully.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func removeOpcodeRaw(script []byte, opcode byte) []byte {
	// Avoid work when possible.
	if len(script) == 0 {
		return script
	}

	const scriptVersion = 0
	var result []byte
	var prevOffset int32

	tokenizer := MakeScriptTokenizer(scriptVersion, script)
	for tokenizer.Next() {
		if tokenizer.Opcode() == opcode {
			if result == nil {
				result = make([]byte, 0, len(script))
				result = append(result, script[:prevOffset]...)
			}
		} else if result != nil {
			result = append(result, script[prevOffset:tokenizer.ByteIndex()]...)
		}
		prevOffset = tokenizer.ByteIndex()
	}
	if result == nil {
		return script
	}
	return result
}

// isCanonicalPush returns true if the opcode is either not a push instruction
// or the data associated with the push instruction uses the smallest
// instruction to do the job.  False otherwise.
//
// For example, it is possible to push a value of 1 to the stack as "OP_1",
// "OP_DATA_1 0x01", "OP_PUSHDATA1 0x01 0x01", and others, however, the first
// only takes a single byte, while the rest take more.  Only the first is
// considered canonical.
func isCanonicalPush(opcode byte, data []byte) bool {
	dataLen := len(data)
	if opcode > OP_16 {
		return true
	}

	if opcode < OP_PUSHDATA1 && opcode > OP_0 && (dataLen == 1 && data[0] <= 16) {
		return false
	}
	if opcode == OP_PUSHDATA1 && dataLen < OP_PUSHDATA1 {
		return false
	}
	if opcode == OP_PUSHDATA2 && dataLen <= 0xff {
		return false
	}
	if opcode == OP_PUSHDATA4 && dataLen <= 0xffff {
		return false
	}
	return true
}

// removeOpcodeByData will return the script minus any opcodes that perform a
// canonical push of data that contains the passed data to remove.  This
// function assumes it is provided a version 0 script as any future version of
// script should avoid this functionality since it is unnecessary due to the
// signature scripts not being part of the witness-free transaction hash.
//
// WARNING: This will return the passed script unmodified unless a modification
// is necessary in which case the modified script is returned.  This implies
// callers may NOT rely on being able to safely mutate either the passed or
// returned script without potentially modifying the same data.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func removeOpcodeByData(script []byte, dataToRemove []byte) ([]byte, bool) {
	// Avoid work when possible.
	if len(script) == 0 || len(dataToRemove) == 0 {
		return script, false
	}

	// Parse through the script looking for a canonical data push that contains
	// the data to remove.
	const scriptVersion = 0
	var result []byte
	var prevOffset int32
	var match bool
	tokenizer := MakeScriptTokenizer(scriptVersion, script)
	for tokenizer.Next() {
		var found bool
		result, prevOffset, found = removeOpcodeCanonical(
			&tokenizer, script, dataToRemove, prevOffset, result,
		)
		if found {
			match = true
		}
	}
	if result == nil {
		result = script
	}
	return result, match
}

func removeOpcodeCanonical(t *ScriptTokenizer, script, dataToRemove []byte,
	prevOffset int32, result []byte) ([]byte, int32, bool) {

	var found bool

	// In practice, the script will basically never actually contain the
	// data since this function is only used during signature verification
	// to remove the signature itself which would require some incredibly
	// non-standard code to create.
	//
	// Thus, as an optimization, avoid allocating a new script unless there
	// is actually a match that needs to be removed.
	op, data := t.Opcode(), t.Data()
	if isCanonicalPush(op, data) && bytes.Equal(data, dataToRemove) {
		if result == nil {
			fullPushLen := t.ByteIndex() - prevOffset
			result = make([]byte, 0, int32(len(script))-fullPushLen)
			result = append(result, script[0:prevOffset]...)
		}
		found = true
	} else if result != nil {
		result = append(result, script[prevOffset:t.ByteIndex()]...)
	}

	return result, t.ByteIndex(), found
}

// AsSmallInt returns the passed opcode, which must be true according to
// IsSmallInt(), as an integer.
func AsSmallInt(op byte) int {
	if op == OP_0 {
		return 0
	}

	return int(op - (OP_1 - 1))
}

// checkScriptParses returns an error if the provided script fails to parse.
func checkScriptParses(scriptVersion uint16, script []byte) error {
	tokenizer := MakeScriptTokenizer(scriptVersion, script)
	for tokenizer.Next() {
		// Nothing to do.
	}
	return tokenizer.Err()
}

// IsUnspendable returns whether the passed public key script is unspendable, or
// guaranteed to fail at execution.  This allows outputs to be pruned instantly
// when entering the UTXO set.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func IsUnspendable(pkScript []byte) bool {
	// The script is unspendable if starts with OP_RETURN or is guaranteed
	// to fail at execution due to being larger than the max allowed script
	// size.
	switch {
	case len(pkScript) > 0 && pkScript[0] == OP_RETURN:
		return true
	case len(pkScript) > MaxScriptSize:
		return true
	}

	// The script is unspendable if it is guaranteed to fail at execution.
	const scriptVersion = 0
	return checkScriptParses(scriptVersion, pkScript) != nil
}

// ScriptHasOpSuccess returns true if any op codes in the script contain an
// OP_SUCCESS op code.
func ScriptHasOpSuccess(witnessScript []byte) bool {
	// First, create a new script tokenizer so we can run through all the
	// elements.
	tokenizer := MakeScriptTokenizer(0, witnessScript)

	// Run through all the op codes, returning true if we find anything
	// that is marked as a new op success.
	for tokenizer.Next() {
		if _, ok := successOpcodes[tokenizer.Opcode()]; ok {
			return true
		}
	}

	return false
}
