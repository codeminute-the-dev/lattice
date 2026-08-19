// Copyright (c) 2013, 2014 The Pearl Research Labs
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// AmountUnit describes a method of converting an Amount to something
// other than the base unit of a lattice.  The value of the AmountUnit
// is the exponent component of the decadic multiple to convert from
// an amount in lattices to an amount counted in units.
type AmountUnit int

// These constants define various units used when describing a lattice
// monetary amount.
const (
	AmountMegaLATT  AmountUnit = 6
	AmountKiloLATT  AmountUnit = 3
	AmountLATT      AmountUnit = 0
	AmountMilliLATT AmountUnit = -3
	AmountMicroLATT AmountUnit = -6
	AmountCell      AmountUnit = -8
)

// String returns the unit as a string.  For recognized units, the SI
// prefix is used, or "Cell" for the base unit.  For all unrecognized
// units, "1eN LATT" is returned, where N is the AmountUnit.
func (u AmountUnit) String() string {
	switch u {
	case AmountMegaLATT:
		return "MLATT"
	case AmountKiloLATT:
		return "kLATT"
	case AmountLATT:
		return "LATT"
	case AmountMilliLATT:
		return "mLATT"
	case AmountMicroLATT:
		return "μLATT"
	case AmountCell:
		return "Cell"
	default:
		return "1e" + strconv.FormatInt(int64(u), 10) + " LATT"
	}
}

// Amount represents the base monetary unit (colloquially referred
// to as a `Cell').  A single Amount is equal to 1e-8 of a lattice.
type Amount int64

// round converts a floating point number, which may or may not be representable
// as an integer, to the Amount integer type by rounding to the nearest integer.
// This is performed by adding or subtracting 0.5 depending on the sign, and
// relying on integer truncation to round the value to the nearest Amount.
func round(f float64) Amount {
	if f < 0 {
		return Amount(f - 0.5)
	}
	return Amount(f + 0.5)
}

// NewAmount creates an Amount from a floating point value representing
// some value in lattices.  NewAmount errors if f is NaN or +-Infinity, but
// does not check that the amount is within the total amount of lattices
// producible as f may not refer to an amount at a single moment in time.
//
// NewAmount is specifically for converting LATT to Cell.
// For creating a new Amount with an int64 value which denotes a quantity of Cell,
// do a simple type conversion from type int64 to Amount.
// See GoDoc for example: https://pkg.go.dev/github.com/codeminute-the-dev/lattice/node/btcutil#example-Amount
func NewAmount(f float64) (Amount, error) {
	// The amount is only considered invalid if it cannot be represented
	// as an integer type.  This may happen if f is NaN or +-Infinity.
	switch {
	case math.IsNaN(f):
		fallthrough
	case math.IsInf(f, 1):
		fallthrough
	case math.IsInf(f, -1):
		return 0, errors.New("invalid lattice amount")
	}

	return round(f * CellPerLatt), nil
}

// ToUnit converts a monetary amount counted in base units to a
// floating point value representing an amount of lattices.
func (a Amount) ToUnit(u AmountUnit) float64 {
	return float64(a) / math.Pow10(int(u+8))
}

// ToLATT is the equivalent of calling ToUnit with AmountLATT.
func (a Amount) ToLATT() float64 {
	return a.ToUnit(AmountLATT)
}

// Format formats a monetary amount counted in base units as a
// string for a given unit.  The conversion will succeed for any unit,
// however, known units will be formatted with an appended label describing
// the units with SI notation, or "Cell" for the base unit.
func (a Amount) Format(u AmountUnit) string {
	units := " " + u.String()
	formatted := strconv.FormatFloat(a.ToUnit(u), 'f', -int(u+8), 64)

	// When formatting full LATT, add trailing zeroes for numbers
	// with decimal point to ease reading of cell amount.
	if u == AmountLATT {
		if strings.Contains(formatted, ".") {
			return fmt.Sprintf("%.8f%s", a.ToUnit(u), units)
		}
	}
	return formatted + units
}

// String is the equivalent of calling Format with AmountLATT.
func (a Amount) String() string {
	return a.Format(AmountLATT)
}

// MulF64 multiplies an Amount by a floating point value.  While this is not
// an operation that must typically be done by a full node or wallet, it is
// useful for services that build on top of Lattice (for example, calculating
// a fee by multiplying by a percentage).
func (a Amount) MulF64(f float64) Amount {
	return round(float64(a) * f)
}
