package main

import (
	"log"
	"net/http"
	"os"
	"fmt"
	"strings"

	"github.com/joho/godotenv"
	"github.com/gin-gonic/gin"
	"github.com/slack-go/slack"
)


type Config struct {
	SlackTeamID string
	SlackAccessToken string
	SlackStatusChannelID string
	SlackBotID string
	OrgName string
}

var config Config

var statusHistory []slack.Message

func init() {
	// Load environment variables, one way or another
	err := godotenv.Load()
	if err != nil {
		log.Println("Couldn't load .env file")
	}

	config.SlackTeamID = os.Getenv("CSP_SLACK_TEAMID")
	config.SlackAccessToken = os.Getenv("CSP_SLACK_ACCESS_TOKEN")
	config.SlackStatusChannelID = os.Getenv("CSP_SLACK_STATUS_CHANNEL")
	config.OrgName = os.Getenv("CSP_ORG_NAME")

	statusHistory, err = getStatusHistory()
	if err != nil {
		log.Println(err)
	}

	api := slack.New(config.SlackAccessToken)
	authTestResponse, err := api.AuthTest()
	config.SlackBotID = authTestResponse.UserID
}
// TODO: Have a (global?) slack client

func statusPage(c *gin.Context) {
	var msgs []string
	for _, message := range statusHistory {
		teamID := fmt.Sprintf("<@%s>", config.SlackBotID)
		if strings.Contains(message.Text, teamID) {
			text := strings.Replace(message.Text, teamID, "", -1)
			msgs = append(msgs, text)
		}
	}

	c.HTML(http.StatusOK, "index.html", gin.H{"Messages" : msgs, "Org" : config.OrgName})
}

func main() {
	app := gin.Default()
	app.LoadHTMLGlob("templates/*")
	app.Static("/static", "./static")

	slackGroup := app.Group("/slack")

	// Serve initial interactions with the bot
	eventGroup := slackGroup.Group("/event")
	eventGroup.Use(signatureVerification)
	eventGroup.POST("/handle", eventResp())

	interactionGroup := slackGroup.Group("/interaction")
	interactionGroup.Use(signatureVerification)
	interactionGroup.POST("/handle", interactionResp())

	app.GET("/", statusPage)

	_ = app.Run()
}
