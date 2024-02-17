package main

import (
	"github.com/gin-gonic/gin"
)

// GET /v2/status
// This is a very simple call which returns an OK status if the user has been authenticated successfully
func statusHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": "ok",
	})
}
