package web

import (
	"fmt"
	"net/http"
	"strconv"

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

func AddFile(c *gin.Context) {
	var newFile File

	if err := c.BindJSON(&newFile); err != nil {
		return
	}

	for _, file := range files {
		if file.ID == newFile.ID {
			message := fmt.Sprintf("A file with ID %v already exists.", file.ID)
			c.IndentedJSON(http.StatusConflict, gin.H{"message": message})
			return
		}
	}

	files = append(files, newFile)
	c.IndentedJSON(http.StatusCreated, newFile)
}

func GetFile(c *gin.Context) {
	fileID, _ := strconv.Atoi(c.Param("id"))

	for _, file := range files {
		if file.ID == fileID {
			c.IndentedJSON(http.StatusFound, file)
			return
		}
	}

	c.IndentedJSON(http.StatusConflict, gin.H{"message": fmt.Sprintf("No file with ID %v found.", fileID)})
}
