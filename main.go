package main

import (
	"log"

	"github.com/teliaz/dot-vault/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
