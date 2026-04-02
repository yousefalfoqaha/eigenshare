package utils

import (
	"errors"
	"fmt"
)

func Greeting(name string) (string, error) {
	if name == "" {
		return name, errors.New("Name cannot be empty.")
	}

	greeting := fmt.Sprintf("Hello %v, how've you been?", name)
	return greeting, nil
}
