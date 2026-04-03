package main

import (
	"eigenshare/backend/web"
	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	router.GET("/files", web.GetFiles)
	router.POST("/files", web.AddFile)
	router.GET("/files/:id", web.GetFile)

	router.Run("localhost:8080")
}
