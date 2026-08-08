// Package controllers 包含 logs 服务端的公共控制器（根路径等）。package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Main 返回一行纯文本，表示 logs 服务端存活。
func Main(c *gin.Context) {
	c.String(http.StatusOK, "Ydsz Trace logs server is running.")
}
