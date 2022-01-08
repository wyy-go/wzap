package main

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wyy-go/wzap"
	"go.uber.org/zap"
)

func main() {
	r := gin.New()

	logger, _ := zap.NewProduction()

	// Add a ginzap middleware, which:
	//   - Logs all requests, like a combined access and error log.
	//   - Logs to stdout.
	//   - RFC3339 with UTC time format.
	r.Use(wzap.New(
		wzap.WithZapLogger(logger),
		wzap.WithUTC(true),
		wzap.WithTimeFormat(time.RFC3339),
		wzap.WithCustomFields(
			func(c *gin.Context) zap.Field { return zap.String("custom field1", c.ClientIP()) },
			func(c *gin.Context) zap.Field { return zap.Any("custom field2", c.ClientIP()) },
		),
		wzap.WithSkipPaths("/skip1"),
		wzap.WithSkip(func(c *gin.Context) bool {
			return c.Request.URL.Path == "/skip2"
		}),
	))

	// Logs all panic to error log
	//   - stack means whether output the stack info.
	r.Use(wzap.Recovery(
		wzap.WithZapLogger(logger),
		wzap.WithStack(true),
		wzap.WithCustomFields(
			func(c *gin.Context) zap.Field { return zap.String("custom field1", c.ClientIP()) },
			func(c *gin.Context) zap.Field { return zap.Any("custom field2", c.ClientIP()) },
		),
	))

	// Example ping request.
	r.GET("/ping", func(c *gin.Context) {
		c.String(200, "pong "+fmt.Sprint(time.Now().Unix()))
	})

	// Example when panic happen.
	r.GET("/panic", func(c *gin.Context) {
		panic("An unexpected error happen!")
	})

	r.GET("/skip1", func(c *gin.Context) {
		c.String(200,"skip1!")
	})

	r.GET("/skip2", func(c *gin.Context) {
		c.String(200,"skip2!")
	})

	// Listen and Server in 0.0.0.0:8080
	r.Run(":8080")
}
