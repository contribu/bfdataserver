package main

import (
	"log"
	"os"
	"github.com/gin-gonic/gin"
	"github.com/urfave/cli/v2"
)

func initializeTicker(r *gin.Engine) {
	// 定期的にticker取得
	// websocketと混ぜる
	// executionsやbookとも混ぜる

	r.GET("/ticker", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})
}

func main() {
	app := &cli.App{
		Name: "bfdataserver",
		Usage: "fast access to bitflyer data",
		Action: func(c *cli.Context) error {
			r := gin.Default()
			initializeTicker(r)
			r.Run("localhost:8080")
			return nil
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
