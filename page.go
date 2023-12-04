package main

import (
	"html/template"
	"net/http"

	"github.com/gin-gonic/gin"
)

type StatusUpdate struct {
	Text            string
	SentBy          string
	TimeStamp       string
	BackgroundClass string
	IconFilename    string
}

// Nothing technically implements this interface, apparently. Go structs are
// an S-Tier Shindogu.
type CSPService interface {
	BuildStatusPage() error
	Run()
	SendReminders() error
}

type CSPPage struct {
	updates []StatusUpdate
	pinnedUpdates []StatusUpdate
}


func (page *CSPPage) statusPage(c *gin.Context) {
	c.HTML(
		http.StatusOK,
		"index.html",
		gin.H{
			"HelpMessage":    template.HTML(config.HelpMessage),
			"PinnedStatuses": page.pinnedUpdates,
			"StatusUpdates":  page.updates,
			"Org":            config.OrgName,
			"Logo":           config.LogoURL,
			"Favicon":        config.FaviconURL,
			"NominalMessage": config.NominalMessage,
		},
	)
}

func health(c *gin.Context) {
	c.JSON(http.StatusOK, "cursed-status-page")
}
