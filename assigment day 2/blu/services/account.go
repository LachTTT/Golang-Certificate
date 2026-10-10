package services

import (
	"blu/repository/account"
	"errors"
	"math/big"
	"strings"
)

func CreateAccount(name string, balance *big.Float) error {
	name = strings.TrimSpace(name)

	if name == "" {
		return errors.New("[ERROR] account name cannot be empty")
	}

	if balance == nil || balance.Cmp(big.NewFloat(0)) < 0 {
		return errors.New("[ERROR] balance must be a non-negative value")
	}

	// Prevent duplicate account
	_, _, err := account.FindByName(name)
	if err == nil {
		return errors.New("[ERROR] account already exists")
	}

	return account.Save(name, new(big.Float).Copy(balance))
}

func RemoveAccount(name string) error {
	name = strings.TrimSpace(name)

	if name == "" {
		return errors.New("[ERROR] account name cannot be empty")
	}

	return account.DeleteByName(name)
}
