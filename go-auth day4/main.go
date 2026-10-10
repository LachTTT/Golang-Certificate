package main

import (
	"fmt"
	"net/http"
)

func main() {
	seedUser()

	http.HandleFunc("/api/auth/login", loginHandler)

	fmt.Println("Server running on :8080")

	err := http.ListenAndServe(":8080", nil)

	if err != nil {
		panic(err)
	}
}
