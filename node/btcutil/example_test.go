package btcutil_test

import (
	"fmt"
	"math"

	"github.com/codeminute-the-dev/lattice/node/btcutil"
)

func ExampleAmount() {

	a := btcutil.Amount(0)
	fmt.Println("Zero Cell:", a)

	a = btcutil.Amount(1e8)
	fmt.Println("100,000,000 Cells:", a)

	a = btcutil.Amount(1e5)
	fmt.Println("100,000 Cells:", a)
	// Output:
	// Zero Cell: 0 LATT
	// 100,000,000 Cells: 1 LATT
	// 100,000 Cells: 0.00100000 LATT
}

func ExampleNewAmount() {
	amountOne, err := btcutil.NewAmount(1)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(amountOne) //Output 1

	amountFraction, err := btcutil.NewAmount(0.01234567)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(amountFraction) //Output 2

	amountZero, err := btcutil.NewAmount(0)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(amountZero) //Output 3

	amountNaN, err := btcutil.NewAmount(math.NaN())
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(amountNaN) //Output 4

	// Output: 1 LATT
	// 0.01234567 LATT
	// 0 LATT
	// invalid lattice amount
}

func ExampleAmount_unitConversions() {
	amount := btcutil.Amount(44433322211100)

	fmt.Println("Cell to kLATT:", amount.Format(btcutil.AmountKiloLATT))
	fmt.Println("Cell to LATT:", amount)
	fmt.Println("Cell to MilliLATT:", amount.Format(btcutil.AmountMilliLATT))
	fmt.Println("Cell to MicroLATT:", amount.Format(btcutil.AmountMicroLATT))
	fmt.Println("Cell to Cell:", amount.Format(btcutil.AmountCell))

	// Output:
	// Cell to kLATT: 444.333222111 kLATT
	// Cell to LATT: 444333.22211100 LATT
	// Cell to MilliLATT: 444333222.111 mLATT
	// Cell to MicroLATT: 444333222111 μLATT
	// Cell to Cell: 44433322211100 Cell
}
