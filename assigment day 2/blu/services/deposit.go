package services

import (
	"blu/repository/account"
	"blu/repository/deposit"
	"fmt"
	"math"
	"math/big"
	"time"
)

var interestRate = big.NewFloat(0.01)

func AddDeposit(name string, amount *big.Float) error {
	if amount == nil || amount.Cmp(big.NewFloat(0)) <= 0 {
		return fmt.Errorf("invalid amount format")
	}

	_, _, err := account.FindByName(name)
	if err != nil {
		return err
	}

	return deposit.Save(name, new(big.Float).Copy(amount))
}

func ShowDeposits() error {
	deposits, err := deposit.FindAll()
	if err != nil {
		return err
	}

	fmt.Println("Current deposits:")

	for i, d := range deposits {
		name := d["name"].(string)
		amount := d["amount"].(*big.Float)
		created := d["created_at"].(time.Time)

		fmt.Printf(
			"%d. %s: %s (created %s)\n",
			i+1,
			name,
			amount.Text('f', 2),
			created.Format(time.RFC822),
		)
	}

	return nil
}

func AccrueInterest() error {
	deposits, err := deposit.FindAll()
	if err != nil {
		return err
	}

	now := time.Now()

	for _, d := range deposits {
		amount := d["amount"].(*big.Float)

		createdAt, ok := d["created_at"].(time.Time)
		if !ok {
			continue
		}

		// Calculate full elapsed minutes
		minutesPassed := int(now.Sub(createdAt).Minutes())

		if minutesPassed <= 0 {
			continue
		}

		// Compound interest:
		// amount = amount * (1 + rate)^minutes
		rate := new(big.Float).Add(
			big.NewFloat(1),
			interestRate,
		)

		// big.Float doesn't have Pow,
		// so calculate using math.Pow and convert the result.
		factor := math.Pow(
			rateValue(rate),
			float64(minutesPassed),
		)

		factorBig := big.NewFloat(factor)

		newAmount := new(big.Float).Mul(
			amount,
			factorBig,
		)

		d["amount"] = newAmount

		// Important:
		// Move timestamp forward only by the number
		// of whole minutes that were actually accrued.
		d["created_at"] = createdAt.Add(
			time.Duration(minutesPassed) * time.Minute,
		)
	}

	return deposit.SaveAll(deposits)
}

func rateValue(value *big.Float) float64 {
	result, _ := value.Float64()
	return result
}
