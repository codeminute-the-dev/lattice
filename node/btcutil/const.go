// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil

const (
	// CellPerLattCent is the number of cells in one lattice cent.
	CellPerLattCent = 1e6

	// CellPerLatt is the number of cells in one lattice (1 LATT).
	CellPerLatt = 1e8

	// MaxCell is the maximum transaction amount allowed in cells.
	MaxCell = 21e9 * CellPerLatt
)
