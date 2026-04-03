package utils

import (
	"errors"
	"fmt"
	"math/rand"
)

func Greeting(name string) (string, error) {
	if name == "" {
		return name, errors.New("Name cannot be empty.")
	}

	greeting := fmt.Sprintf(randomGreeting(), name)
	return greeting, nil
}

func randomGreeting() string {
	greetings := []string{
		"Hello %v, hope everything's well!",
		"Sup ma boy %v, everything cool yeah?",
		"Kill yourself %v... NOT! HAH!",
	}

	return greetings[rand.Intn(len(greetings))]
}
