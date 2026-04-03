package main

import (
	"eigenshare/backend/web"
	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	router.GET("/files", web.GetFiles)

	router.Run("localhost:8080")
}
