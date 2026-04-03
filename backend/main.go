package main

import (
	"eigenshare/backend/utils"
	"fmt"
	"log"
)

func main() {
	greeting, err := utils.Greeting("Yousef")
	if err != nil {
		log.Fatal("Error calling function")
	}

	fmt.Println(greeting)
}
