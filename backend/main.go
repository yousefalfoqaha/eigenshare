package main

import (
	"fmt"

	"eigenshare/backend/web"
)

func main() {
	f1 := &web.File{Title: "hello", Body: []byte("Yessir")}
	f1.Save()

	file, _ := f1.Load("hello")
	fmt.Println(string(file.Body))

	// router := gin.Default()
	//
	// router.GET("/files", web.GetGinFiles)
	// router.POST("/files", web.AddGinFile)
	// router.GET("/files/:id", web.GetGinFile)
	//
	// router.Run("localhost:8080")
}
