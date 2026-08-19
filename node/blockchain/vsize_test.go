// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import "testing"

// TestVsizeCalculation tests the vsize calculation formula with
// parametrized baseSize and witnessSize values.
// Formula: vsize = baseSize + ceiling(witnessSize / WitnessScaleFactor)
func TestVsizeCalculation(t *testing.T) {
	tests := []struct {
		name        string
		baseSize    int
		witnessSize int
		expected    int64
	}{
		// No witness data cases
		{
			name:        "no witness - small tx",
			baseSize:    100,
			witnessSize: 0,
			expected:    100,
		},
		{
			name:        "no witness - medium tx",
			baseSize:    250,
			witnessSize: 0,
			expected:    250,
		},
		{
			name:        "no witness - large tx",
			baseSize:    1000,
			witnessSize: 0,
			expected:    1000,
		},

		// Witness size exactly divisible by 4
		{
			name:        "witness divisible by 4 - 4 bytes",
			baseSize:    100,
			witnessSize: 4,
			expected:    101, // 100 + 4/4 = 101
		},
		{
			name:        "witness divisible by 4 - 8 bytes",
			baseSize:    100,
			witnessSize: 8,
			expected:    102, // 100 + 8/4 = 102
		},
		{
			name:        "witness divisible by 4 - 100 bytes",
			baseSize:    200,
			witnessSize: 100,
			expected:    225, // 200 + 100/4 = 225
		},
		{
			name:        "witness divisible by 4 - 400 bytes",
			baseSize:    200,
			witnessSize: 400,
			expected:    300, // 200 + 400/4 = 300
		},
		{
			name:        "witness divisible by 4 - 4000 bytes",
			baseSize:    1000,
			witnessSize: 4000,
			expected:    2000, // 1000 + 4000/4 = 2000
		},

		// Witness size needs ceiling - test all remainders (1, 2, 3)
		{
			name:        "witness needs ceiling - 1 byte (remainder 1)",
			baseSize:    100,
			witnessSize: 1,
			expected:    101, // 100 + ceiling(1/4) = 100 + 1 = 101
		},
		{
			name:        "witness needs ceiling - 2 bytes (remainder 2)",
			baseSize:    100,
			witnessSize: 2,
			expected:    101, // 100 + ceiling(2/4) = 100 + 1 = 101
		},
		{
			name:        "witness needs ceiling - 3 bytes (remainder 3)",
			baseSize:    100,
			witnessSize: 3,
			expected:    101, // 100 + ceiling(3/4) = 100 + 1 = 101
		},
		{
			name:        "witness needs ceiling - 5 bytes (remainder 1)",
			baseSize:    100,
			witnessSize: 5,
			expected:    102, // 100 + ceiling(5/4) = 100 + 2 = 102
		},
		{
			name:        "witness needs ceiling - 6 bytes (remainder 2)",
			baseSize:    100,
			witnessSize: 6,
			expected:    102, // 100 + ceiling(6/4) = 100 + 2 = 102
		},
		{
			name:        "witness needs ceiling - 7 bytes (remainder 3)",
			baseSize:    100,
			witnessSize: 7,
			expected:    102, // 100 + ceiling(7/4) = 100 + 2 = 102
		},
		{
			name:        "witness needs ceiling - 9 bytes (remainder 1)",
			baseSize:    100,
			witnessSize: 9,
			expected:    103, // 100 + ceiling(9/4) = 100 + 3 = 103
		},
		{
			name:        "witness needs ceiling - 101 bytes (remainder 1)",
			baseSize:    200,
			witnessSize: 101,
			expected:    226, // 200 + ceiling(101/4) = 200 + 26 = 226
		},
		{
			name:        "witness needs ceiling - 401 bytes (remainder 1)",
			baseSize:    200,
			witnessSize: 401,
			expected:    301, // 200 + ceiling(401/4) = 200 + 101 = 301
		},

		// Typical real-world transaction scenarios
		{
			name:        "typical P2WPKH - 1 input 2 outputs",
			baseSize:    110,
			witnessSize: 109,
			expected:    138, // 110 + ceiling(109/4) = 110 + 28 = 138
		},
		{
			name:        "typical P2WSH multisig",
			baseSize:    150,
			witnessSize: 220,
			expected:    205, // 150 + ceiling(220/4) = 150 + 55 = 205
		},
		{
			name:        "large segwit transaction",
			baseSize:    500,
			witnessSize: 1000,
			expected:    750, // 500 + ceiling(1000/4) = 500 + 250 = 750
		},

		// Block-level scenarios
		{
			name:        "block with 50% base, 50% witness",
			baseSize:    500000,
			witnessSize: 2000000,
			expected:    1000000, // 500000 + ceiling(2000000/4) = 500000 + 500000 = 1000000
		},
		{
			name:        "block at max vsize - mostly base",
			baseSize:    800000,
			witnessSize: 800000,
			expected:    1000000, // 800000 + ceiling(800000/4) = 800000 + 200000 = 1000000
		},
		{
			name:        "block at max vsize - base only",
			baseSize:    MaxBlockVsize,
			witnessSize: 0,
			expected:    MaxBlockVsize,
		},
		{
			name:        "block with mostly witness data",
			baseSize:    80,
			witnessSize: 3999920,
			expected:    1000060, // 80 + ceiling(3999920/4) = 80 + 999980 = 1000060
		},

		// Edge cases
		{
			name:        "zero base with witness",
			baseSize:    0,
			witnessSize: 4,
			expected:    1, // 0 + ceiling(4/4) = 1
		},
		{
			name:        "zero base with witness needing ceiling",
			baseSize:    0,
			witnessSize: 3,
			expected:    1, // 0 + ceiling(3/4) = 1
		},
		{
			name:        "zero base with witness needing ceiling - 1 byte",
			baseSize:    0,
			witnessSize: 1,
			expected:    1, // 0 + ceiling(1/4) = 1
		},
		{
			name:        "completely empty",
			baseSize:    0,
			witnessSize: 0,
			expected:    0,
		},
		{
			name:        "very large witness - 3MB",
			baseSize:    100,
			witnessSize: 3000000,
			expected:    750100, // 100 + ceiling(3000000/4) = 100 + 750000 = 750100
		},
		{
			name:        "very large witness with ceiling - 3000001 bytes",
			baseSize:    100,
			witnessSize: 3000001,
			expected:    750101, // 100 + ceiling(3000001/4) = 100 + 750001 = 750101
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := CalcVsize(tt.baseSize, tt.witnessSize)
			if actual != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, actual)
			}
		})
	}
}
