package main

import (
	"fmt"
	"log"

	"eigenshare/backend/utils"
)

func main() {
	greeting, err := utils.Greeting("Yousef")
	if err != nil {
		log.Fatal("Error calling function")
	}

	fmt.Println(greeting)
}
