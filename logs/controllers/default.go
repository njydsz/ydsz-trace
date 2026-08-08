package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Main 根路径返回服务信息
func Main(c *gin.Context) {
	c.String(http.StatusOK, "Ydsz Trace logs server is running.")
}
