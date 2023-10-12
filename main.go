package main

import (
	"log"
	"net/http"
	"os"
	"fmt"
	"strings"
	"time"
	"strconv"

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
	StatusNeutralColor string
	StatusOKColor string
	StatusOKEmoji string
	StatusWarnColor string
	StatusWarnEmoji string
	StatusErrorColor string
	StatusErrorEmoji string
}

type StatusUpdate struct {
	Text string
	SentBy string
	TimeStamp string
	Background string
}

var config Config

var statusHistory []slack.Message

var slackAPI *slack.Client

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

	config.StatusNeutralColor = os.Getenv("CSP_CARD_NEUTRAL_COLOR")
	config.StatusOKColor = os.Getenv("CSP_CARD_OK_COLOR")
	config.StatusOKEmoji= os.Getenv("CSP_CARD_OK_EMOJI")
	config.StatusWarnColor= os.Getenv("CSP_CARD_WARN_COLOR")
	config.StatusWarnEmoji= os.Getenv("CSP_CARD_WARN_EMOJI")
	config.StatusErrorColor= os.Getenv("CSP_CARD_ERROR_COLOR")
	config.StatusErrorEmoji= os.Getenv("CSP_CARD_ERROR_EMOJI")

	statusHistory, err = getStatusHistory()
	if err != nil {
		log.Println(err)
	}

	slackAPI = slack.New(config.SlackAccessToken)
	authTestResponse, err := slackAPI.AuthTest()
	config.SlackBotID = authTestResponse.UserID
}
// TODO: Have a (global?) slack client

func slackTSToHumanTime(slackTimestamp string) (hrt string) {
	// Convert the Slack timestamp to a Unix timestamp (float64)
	slackUnixTimestamp, err := strconv.ParseFloat(strings.Split(slackTimestamp, ".")[0], 64)
	if err != nil {
		fmt.Println("Error parsing Slack timestamp:", err)
		return
	}

	// Create a time.Time object from the Unix timestamp (assuming UTC time zone)
	slackTime := time.Unix(int64(slackUnixTimestamp), 0)

	// Convert to a specific time zone (e.g., "America/New_York")
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		fmt.Println("Error loading location:", err)
		return
	}

	slackTimeInLocation := slackTime.In(location)

	// Format the time as a human-readable string
	humanReadableTimestamp := slackTimeInLocation.Format("2006-01-02 15:04:05 MST")

	return humanReadableTimestamp
}

func statusPage(c *gin.Context) {
	var updates []StatusUpdate
	for _, message := range statusHistory {
		teamID := fmt.Sprintf("<@%s>", config.SlackBotID)
		if strings.Contains(message.Text, teamID) {
			chom := slack.New(config.SlackAccessToken)
			msgUser, err := chom.GetUserInfo(message.User)
			if err != nil {
				fmt.Println(err)
				c.String(http.StatusInternalServerError, "error reading request body: %s", err.Error())
			}
			realName := msgUser.RealName
			var update StatusUpdate
			update.Text = strings.Replace(message.Text, teamID, "", -1)
			update.SentBy = realName
			update.TimeStamp = slackTSToHumanTime(message.Timestamp) 
			update.Background = config.StatusNeutralColor 

			for _, reaction := range message.Reactions {
				if reaction.Name == config.StatusOKEmoji {
					update.Background = config.StatusOKColor
					break
				} else if reaction.Name == config.StatusWarnEmoji {
					update.Background = config.StatusWarnColor 
					break
				} else if reaction.Name == config.StatusErrorEmoji {
					update.Background = config.StatusErrorColor 
				}
			}   

			updates = append(updates, update)
		}
	}

	c.HTML(http.StatusOK, "index.html", gin.H{"StatusUpdates" : updates, "Org" : config.OrgName})
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
