package main

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID       int
	Name     string
	Username string
	Password string
}

var users []User

func seedUser() {
	password, err := bcrypt.GenerateFromPassword(
		[]byte("StrongPassword"),
		10,
	)

	if err != nil {
		panic(err)
	}

	users = append(users, User{
		ID:       1,
		Name:     "User 1",
		Username: "user1",
		Password: string(password),
	})

	fmt.Println("User seeded")
}
