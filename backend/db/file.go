package db

import (
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

func Load(title string) (*File, error) {
	filename := title + ".txt"
	body, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf(fmt.Sprintf("Cannot load file with title %v", title))
	}

	return &File{Title: title, Body: body}, nil
}
