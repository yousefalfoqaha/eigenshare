package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type File struct {
	ID   int     `json:"id"`
	Name string  `json:"name"`
	Size float64 `json:"size"`
}

var files = []File{
	{ID: 1, Name: "homework.txt", Size: 3119223.54},
	{ID: 2, Name: "jerash.jpeg", Size: 31.2},
}

func GetFiles(c *gin.Context) {
	c.IndentedJSON(http.StatusOK, files)
}
