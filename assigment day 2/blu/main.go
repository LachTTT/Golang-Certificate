package main

import (
	"fmt"
	"math/big"
	"os"

	"blu/services"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("  create_account <name> <amount>")
		fmt.Println("  transfer <from> <to> <amount>")
		fmt.Println("  add_deposit <name> <amount>")
		fmt.Println("  accrue_interest")
		return
	}

	command := os.Args[1]

	switch command {
	case "create_account":
		if len(os.Args) != 4 {
			fmt.Println("Usage: create_account <name> <amount>")
			return
		}
		name := os.Args[2]
		amount, ok := new(big.Float).SetString(os.Args[3])
		if !ok {
			fmt.Println("Invalid amount")
			return
		}
		if err := services.CreateAccount(name, amount); err != nil {
			panic(err)
		}
		fmt.Println("Account created:", name)

	case "transfer":
		if len(os.Args) != 5 {
			fmt.Println("Usage: transfer <from> <to> <amount>")
			return
		}
		from := os.Args[2]
		to := os.Args[3]
		amount, ok := new(big.Float).SetString(os.Args[4])
		if !ok {
			fmt.Println("Invalid amount")
			return
		}
		if err := services.Transfer(from, to, amount); err != nil {
			panic(err)
		}
		fmt.Println("Transfer successful")

	case "add_deposit":
		if len(os.Args) != 4 {
			fmt.Println("Usage: add_deposit <name> <amount>")
			return
		}
		name := os.Args[2]
		amount, ok := new(big.Float).SetString(os.Args[3])
		if !ok {
			fmt.Println("Invalid amount")
			return
		}
		if err := services.AddDeposit(name, amount); err != nil {
			panic(err)
		}
		fmt.Println("Deposit added")

	case "accrue_interest":
		if err := services.AccrueInterest(); err != nil {
			panic(err)
		}
		fmt.Println("Interest accrued to all deposits")

	default:
		fmt.Println("Unknown command:", command)
	}
}
