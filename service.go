package main

import (
	"github.com/gin-gonic/gin"
)

type CSPService interface {
	BuildStatusPage() error
	StatusPage(gin *gin.Context)
	SendReminders(now bool) error
	Run()
}

type ReminderInfo struct {
	userID string
	link   string
	ts     string
	status string
}
