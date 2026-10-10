package deposit

import (
	"bufio"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"
)

const depositFile = "filebase/deposits.txt"

func Save(name string, amount *big.Float) error {
	f, err := os.OpenFile(depositFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	timestamp := time.Now().Format(time.RFC822)
	line := fmt.Sprintf("%s#%s#%s\n", name, amount.Text('f', 2), timestamp)
	_, err = f.WriteString(line)
	return err
}

func FindAll() ([]map[string]interface{}, error) {
	file, err := os.Open(depositFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []map[string]interface{}{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var deposits []map[string]interface{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Split(line, "#")
		if len(parts) != 3 {
			continue
		}

		amount, _, err := big.ParseFloat(parts[1], 10, 64, big.ToNearestEven)
		if err != nil {
			continue
		}

		timestamp, err := time.Parse(time.RFC822, parts[2])
		if err != nil {
			continue
		}

		deposits = append(deposits, map[string]interface{}{
			"name":       parts[0],
			"amount":     amount,
			"created_at": timestamp,
		})
	}
	return deposits, scanner.Err()
}

func SaveAll(deposits []map[string]interface{}) error {
	var builder strings.Builder
	for _, d := range deposits {
		amount := d["amount"].(*big.Float)
		line := fmt.Sprintf("%s#%s#%s\n",
			d["name"],
			amount.Text('f', 2),
			d["created_at"].(time.Time).Format(time.RFC822),
		)
		builder.WriteString(line)
	}
	return os.WriteFile(depositFile, []byte(builder.String()), 0644)
}
