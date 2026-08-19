// Copyright (c) 2025-2026 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil_test

import (
	"math"
	"testing"

	. "github.com/codeminute-the-dev/lattice/node/btcutil"
)

func TestAmountCreation(t *testing.T) {
	tests := []struct {
		name     string
		amount   float64
		valid    bool
		expected Amount
	}{
		// Positive tests.
		{
			name:     "zero",
			amount:   0,
			valid:    true,
			expected: 0,
		},
		{
			name:     "max producible",
			amount:   21e9,
			valid:    true,
			expected: MaxCell,
		},
		{
			name:     "min producible",
			amount:   -21e9,
			valid:    true,
			expected: -MaxCell,
		},
		{
			name:     "exceeds max producible",
			amount:   21e9 + 1,
			valid:    true,
			expected: MaxCell + 1e8,
		},
		{
			name:     "exceeds min producible",
			amount:   -21e9 - 1,
			valid:    true,
			expected: -MaxCell - 1e8,
		},
		{
			name:     "one hundred",
			amount:   100,
			valid:    true,
			expected: 100 * CellPerLatt,
		},
		{
			name:     "fraction",
			amount:   0.01234567,
			valid:    true,
			expected: 1234567,
		},
		{
			name:     "rounding up",
			amount:   54.999999999999943157,
			valid:    true,
			expected: 55 * CellPerLatt,
		},
		{
			name:     "rounding down",
			amount:   55.000000000000056843,
			valid:    true,
			expected: 55 * CellPerLatt,
		},

		// Negative tests.
		{
			name:   "not-a-number",
			amount: math.NaN(),
			valid:  false,
		},
		{
			name:   "-infinity",
			amount: math.Inf(-1),
			valid:  false,
		},
		{
			name:   "+infinity",
			amount: math.Inf(1),
			valid:  false,
		},
	}

	for _, test := range tests {
		a, err := NewAmount(test.amount)
		switch {
		case test.valid && err != nil:
			t.Errorf("%v: Positive test Amount creation failed with: %v", test.name, err)
			continue
		case !test.valid && err == nil:
			t.Errorf("%v: Negative test Amount creation succeeded (value %v) when should fail", test.name, a)
			continue
		}

		if a != test.expected {
			t.Errorf("%v: Created amount %v does not match expected %v", test.name, a, test.expected)
			continue
		}
	}
}

func TestAmountUnitConversions(t *testing.T) {
	tests := []struct {
		name      string
		amount    Amount
		unit      AmountUnit
		converted float64
		s         string
	}{
		{
			name:      "MLATT",
			amount:    MaxCell,
			unit:      AmountMegaLATT,
			converted: 21000,
			s:         "21000 MLATT",
		},
		{
			name:      "kLATT",
			amount:    44433322211100,
			unit:      AmountKiloLATT,
			converted: 444.33322211100,
			s:         "444.333222111 kLATT",
		},
		{
			name:      "LATT",
			amount:    44433322211100,
			unit:      AmountLATT,
			converted: 444333.222111,
			s:         "444333.22211100 LATT",
		},
		{
			name:      "a thousand cell as LATT",
			amount:    1000,
			unit:      AmountLATT,
			converted: 0.00001,
			s:         "0.00001000 LATT",
		},
		{
			name:      "a single cell as LATT",
			amount:    1,
			unit:      AmountLATT,
			converted: 0.00000001,
			s:         "0.00000001 LATT",
		},
		{
			name:      "amount with trailing zero but no decimals",
			amount:    1000000000,
			unit:      AmountLATT,
			converted: 10,
			s:         "10 LATT",
		},
		{
			name:      "mLATT",
			amount:    44433322211100,
			unit:      AmountMilliLATT,
			converted: 444333222.11100,
			s:         "444333222.111 mLATT",
		},
		{

			name:      "μLATT",
			amount:    44433322211100,
			unit:      AmountMicroLATT,
			converted: 444333222111.00,
			s:         "444333222111 μLATT",
		},
		{

			name:      "cell",
			amount:    44433322211100,
			unit:      AmountCell,
			converted: 44433322211100,
			s:         "44433322211100 Cell",
		},
		{

			name:      "non-standard unit",
			amount:    44433322211100,
			unit:      AmountUnit(-1),
			converted: 4443332.2211100,
			s:         "4443332.22111 1e-1 LATT",
		},
	}

	for _, test := range tests {
		f := test.amount.ToUnit(test.unit)
		if f != test.converted {
			t.Errorf("%v: converted value %v does not match expected %v", test.name, f, test.converted)
			continue
		}

		s := test.amount.Format(test.unit)
		if s != test.s {
			t.Errorf("%v: format '%v' does not match expected '%v'", test.name, s, test.s)
			continue
		}

		// Verify that Amount.ToLATT works as advertised.
		f1 := test.amount.ToUnit(AmountLATT)
		f2 := test.amount.ToLATT()
		if f1 != f2 {
			t.Errorf("%v: ToLATT does not match ToUnit(AmountLATT): %v != %v", test.name, f1, f2)
		}

		// Verify that Amount.String works as advertised.
		s1 := test.amount.Format(AmountLATT)
		s2 := test.amount.String()
		if s1 != s2 {
			t.Errorf("%v: String does not match Format(AmountLATT): %v != %v", test.name, s1, s2)
		}
	}
}

func TestAmountMulF64(t *testing.T) {
	tests := []struct {
		name string
		amt  Amount
		mul  float64
		res  Amount
	}{
		{
			name: "Multiply 0.1 LATT by 2",
			amt:  100e5, // 0.1 LATT
			mul:  2,
			res:  200e5, // 0.2 LATT
		},
		{
			name: "Multiply 0.2 LATT by 0.02",
			amt:  200e5, // 0.2 LATT
			mul:  1.02,
			res:  204e5, // 0.204 LATT
		},
		{
			name: "Multiply 0.1 LATT by -2",
			amt:  100e5, // 0.1 LATT
			mul:  -2,
			res:  -200e5, // -0.2 LATT
		},
		{
			name: "Multiply 0.2 LATT by -0.02",
			amt:  200e5, // 0.2 LATT
			mul:  -1.02,
			res:  -204e5, // -0.204 LATT
		},
		{
			name: "Multiply -0.1 LATT by 2",
			amt:  -100e5, // -0.1 LATT
			mul:  2,
			res:  -200e5, // -0.2 LATT
		},
		{
			name: "Multiply -0.2 LATT by 0.02",
			amt:  -200e5, // -0.2 LATT
			mul:  1.02,
			res:  -204e5, // -0.204 LATT
		},
		{
			name: "Multiply -0.1 LATT by -2",
			amt:  -100e5, // -0.1 LATT
			mul:  -2,
			res:  200e5, // 0.2 LATT
		},
		{
			name: "Multiply -0.2 LATT by -0.02",
			amt:  -200e5, // -0.2 LATT
			mul:  -1.02,
			res:  204e5, // 0.204 LATT
		},
		{
			name: "Round down",
			amt:  49, // 49 Cells
			mul:  0.01,
			res:  0,
		},
		{
			name: "Round up",
			amt:  50, // 50 Cells
			mul:  0.01,
			res:  1, // 1 Cell
		},
		{
			name: "Multiply by 0.",
			amt:  1e8, // 1 LATT
			mul:  0,
			res:  0, // 0 LATT
		},
		{
			name: "Multiply 1 by 0.5.",
			amt:  1, // 1 Cell
			mul:  0.5,
			res:  1, // 1 Cell
		},
		{
			name: "Multiply 100 by 66%.",
			amt:  100, // 100 Cells
			mul:  0.66,
			res:  66, // 66 Cells
		},
		{
			name: "Multiply 100 by 66.6%.",
			amt:  100, // 100 Cells
			mul:  0.666,
			res:  67, // 67 Cells
		},
		{
			name: "Multiply 100 by 2/3.",
			amt:  100, // 100 Cells
			mul:  2.0 / 3,
			res:  67, // 67 Cells
		},
	}

	for _, test := range tests {
		a := test.amt.MulF64(test.mul)
		if a != test.res {
			t.Errorf("%v: expected %v got %v", test.name, test.res, a)
		}
	}
}
