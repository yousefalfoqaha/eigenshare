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

func Greetings(names []string) (map[string]string, error) {
	greetings := make(map[string]string, len(names))

	for _, name := range names {
		greeting, err := Greeting(name)
		if err != nil {
			return nil, err
		}

		greetings[name] = greeting
	}

	return greetings, nil
}

func randomGreeting() string {
	greetings := []string{
		"Hello %v, hope everything's well!",
		"Sup ma boy %v, everything cool yeah?",
		"Kill yourself %v... NOT! HAH!",
	}

	return greetings[rand.Intn(len(greetings))]
}
