package web

import (
	"errors"
	"fmt"
	"os"
)

type File struct {
	Title string `json:"title"`
	Body  []byte `json:"body"`
}

func (f *File) Save() error {
	filename := f.Title + ".txt"
	return os.WriteFile(filename, f.Body, 0o600)
}

func (f *File) Load(title string) (*File, error) {
	filename := title + ".txt"
	body, err := os.ReadFile(filename)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Cannot load file with title %v", title))
	}

	return &File{Title: title, Body: body}, nil
}
