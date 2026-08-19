// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mining

const (
	// UnminedHeight is the height used for the "block" height field of the
	// contextual transaction information provided in a transaction store
	// when it has not yet been mined into a block.
	UnminedHeight = 0x7fffffff
)

// Policy houses the policy (configuration parameters) which is used to control
// the generation of block templates.  See the documentation for
// NewBlockTemplate for more details on each of these parameters are used.
type Policy struct {
	// BlockMinVsize is the minimum block vsize to be used when generating
	// a block template.
	BlockMinVsize uint32

	// BlockMaxVsize is the maximum block vsize to be used when generating a
	// block template.
	BlockMaxVsize uint32
}
