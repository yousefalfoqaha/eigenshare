package main

import (
	"fmt"
	"log"

	"eigenshare/backend/utils"
)

func main() {
	names := []string{"Abdullah", "Abdelkareem", "Abdelmajeed"}

	greetings, err := utils.Greetings(names)
	if err != nil {
		log.Fatalf("Error making greetings, with given names %v", names)
	}

	for _, greeting := range greetings {
		fmt.Println(greeting)
	}
}
