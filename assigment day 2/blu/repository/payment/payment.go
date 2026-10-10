package payment

import (
	"bufio"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"
)

const paymentsFile = "filebase/payments.txt"

func Save(from string, to string, balance *big.Float) error {
	payments, ok := FindAll()
	if ok != nil {
		return fmt.Errorf("[ERROR] failed to retrieve payments: %v", ok)
	}
	payments = append(payments, fmt.Sprintf("%s#%s#%s#%s", from, to, balance.Text('f', 2), time.Now().Format(time.RFC822)))

	return write(payments)
}

func FindAll() ([]string, error) {
	payments := make([]string, 0, 10)

	file, err := os.Open(paymentsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return payments, nil
		}
		return nil, fmt.Errorf("[ERROR] failed to open payments file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		parts := strings.Split(line, "#")
		if len(parts) != 4 {
			continue
		}

		payments = append(payments, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("[ERROR] error scanning payments file: %v", err)
	}

	return payments, nil
}

func write(payments []string) error {
	file, err := os.OpenFile(paymentsFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("[ERROR] failed to open payments file for writing: %v", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, payment := range payments {
		_, err := writer.WriteString(fmt.Sprintf("%s\n", payment))
		if err != nil {
			return fmt.Errorf("[ERROR] failed to write to payments file: %v", err)
		}
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("[ERROR] failed to flush payments file: %v", err)
	}

	return nil
}
