package main

import (
	"github.com/gin-gonic/gin"
)

type CSPService interface {
	BuildStatusPage() error
	StatusPage(gin *gin.Context) 
	SendReminders() error
	Run()
}
