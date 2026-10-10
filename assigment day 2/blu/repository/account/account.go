package account

import (
	"bufio"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
)

const accountsFile = "filebase/accounts.txt"

func Save(name string, balance *big.Float) error {
	accounts, ok := FindAll()
	if ok != nil {
		return fmt.Errorf("[ERROR] failed to retrieve accounts: %v", ok)
	}

	accounts[name] = balance

	return write(accounts)
}

func DeleteByName(name string) error {
	accounts, ok := FindAll()
	if ok != nil {
		return fmt.Errorf("[ERROR] failed to retrieve accounts: %v", ok)
	}

	if _, exists := accounts[name]; exists {
		delete(accounts, name)
	} else {
		return errors.New("[ERROR] account doesn't exist")
	}

	return write(accounts)
}

func FindAll() (map[string]*big.Float, error) {
	accounts := make(map[string]*big.Float)

	file, err := os.Open(accountsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return accounts, nil
		}
		return nil, fmt.Errorf("[ERROR] failed to open accounts file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		parts := strings.Split(line, "#")
		if len(parts) != 2 {
			continue
		}

		balance := new(big.Float)
		if _, ok := balance.SetString(parts[1]); !ok {
			continue
		}
		accounts[parts[0]] = balance
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("[ERROR] error scanning accounts file: %v", err)
	}

	return accounts, nil
}

func FindByName(name string) (string, *big.Float, error) {
	accounts, err := FindAll()

	if err != nil {
		return "", nil, err
	}

	if _, exists := accounts[name]; exists {
		return name, accounts[name], nil
	}

	return "", nil, errors.New("[Error] account doesn't exists")
}

func write(accounts map[string]*big.Float) error {
	file, err := os.OpenFile(accountsFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("[ERROR] failed to open accounts file for writing: %v", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for accountName, accountBalance := range accounts {
		_, err := writer.WriteString(fmt.Sprintf("%s#%s\n", accountName, accountBalance.Text('f', 2)))
		if err != nil {
			return fmt.Errorf("[ERROR] failed to write to accounts file: %v", err)
		}
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("[ERROR] failed to flush accounts file: %v", err)
	}

	return nil
}
