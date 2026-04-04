package web

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type GinFile struct {
	ID   int     `json:"id"`
	Name string  `json:"name"`
	Size float64 `json:"size"`
}

var files = []GinFile{
	{ID: 1, Name: "homework.txt", Size: 3119223.54},
	{ID: 2, Name: "jerash.jpeg", Size: 31.2},
}

func GetGinFiles(c *gin.Context) {
	c.IndentedJSON(http.StatusOK, files)
}

func AddGinFile(c *gin.Context) {
	var newFile GinFile

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

func GetGinFile(c *gin.Context) {
	fileID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": "File ID must be a number."})
		return
	}

	for _, file := range files {
		if file.ID == fileID {
			c.IndentedJSON(http.StatusFound, file)
			return
		}
	}

	c.IndentedJSON(http.StatusConflict, gin.H{"message": fmt.Sprintf("No file with ID %v found.", fileID)})
}
