package services

import (
	"blu/repository/account"
	"blu/repository/payment"
	"errors"
	"math/big"
	"strings"
)

func Transfer(from string, to string, amount *big.Float) error {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)

	if from == "" || to == "" {
		return errors.New("[ERROR] account name cannot be empty")
	}

	if from == to {
		return errors.New("[ERROR] cannot transfer to the same account")
	}

	if amount == nil || amount.Cmp(big.NewFloat(0)) <= 0 {
		return errors.New("[ERROR] transfer amount must be greater than zero")
	}

	fromName, fromBalance, err := account.FindByName(from)
	if err != nil {
		return err
	}

	toName, toBalance, err := account.FindByName(to)
	if err != nil {
		return err
	}

	// Check sufficient balance
	if fromBalance.Cmp(amount) < 0 {
		return errors.New("[ERROR] insufficient balance")
	}

	// Create new values instead of modifying repository values directly
	newFromBalance := new(big.Float).Sub(
		fromBalance,
		amount,
	)

	newToBalance := new(big.Float).Add(
		toBalance,
		amount,
	)

	// Update sender
	if err := account.Save(fromName, newFromBalance); err != nil {
		return err
	}

	// Update receiver
	if err := account.Save(toName, newToBalance); err != nil {
		_ = account.Save(fromName, fromBalance)
		return err
	}

	// Save transaction history
	if err := payment.Save(from, to, amount); err != nil {
		return err
	}

	return nil
}
