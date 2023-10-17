package main

import (
	"log"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
)

type Config struct {
	OrgName    string
	LogoURL    string
	FaviconURL string

	SlackTeamID          string
	SlackAccessToken     string
	SlackStatusChannelID string
	SlackBotID           string
	SlackTruncation      string

	StatusNeutralColor string
	StatusOKColor      string
	StatusOKEmoji      string
	StatusWarnColor    string
	StatusWarnEmoji    string
	StatusErrorColor   string
	StatusErrorEmoji   string

	PinEmoji string
	PinLimit int

	SiteEmoji string

	NominalMessage string
	NominalSentBy  string
	HelpMessage    string
}

type StatusUpdate struct {
	Text       string
	SentBy     string
	TimeStamp  string
	Background string
}

type Site struct {
	Name string
	Background string
}

var config Config

var globalChannelHistory []slack.Message

var globalUpdates []StatusUpdate
var globalPinnedUpdates []StatusUpdate
var globalSites []Site

var slackAPI *slack.Client

func init() {
	// Load environment variables one way or another
	err := godotenv.Load()
	if err != nil {
		log.Println("Couldn't load .env file")
	}

	config.OrgName = os.Getenv("CSP_ORG_NAME")
	config.LogoURL = os.Getenv("CSP_LOGO_URL")
	config.FaviconURL = os.Getenv("CSP_FAVICON_URL")

	config.SlackTeamID = os.Getenv("CSP_SLACK_TEAMID")
	config.SlackAccessToken = os.Getenv("CSP_SLACK_ACCESS_TOKEN")
	config.SlackStatusChannelID = os.Getenv("CSP_SLACK_STATUS_CHANNEL")
	config.SlackTruncation = os.Getenv("CSP_SLACK_TRUNCATION")

	config.StatusNeutralColor = os.Getenv("CSP_CARD_NEUTRAL_COLOR")
	config.StatusOKColor = os.Getenv("CSP_CARD_OK_COLOR")
	config.StatusOKEmoji = os.Getenv("CSP_CARD_OK_EMOJI")
	config.StatusWarnColor = os.Getenv("CSP_CARD_WARN_COLOR")
	config.StatusWarnEmoji = os.Getenv("CSP_CARD_WARN_EMOJI")
	config.StatusErrorColor = os.Getenv("CSP_CARD_ERROR_COLOR")
	config.StatusErrorEmoji = os.Getenv("CSP_CARD_ERROR_EMOJI")

	config.PinEmoji = os.Getenv("CSP_PIN_EMOJI")
	config.PinLimit, _ = strconv.Atoi(os.Getenv("CSP_PIN_LIMIT"))


	config.SiteEmoji = os.Getenv("CSP_SITES_EMOJI")

	config.NominalMessage = os.Getenv("CSP_NOMINAL_MESSAGE")
	config.NominalSentBy = os.Getenv("CSP_NOMINAL_SENT_BY")
	config.HelpMessage = os.Getenv("CSP_HELP_LINK")

	slackAPI = slack.New(config.SlackAccessToken)

	// Get the channel history
	globalChannelHistory, err = getChannelHistory()
	if err != nil {
		log.Fatal(err)
	}

	// Get some deets we'll need from the slack API
	authTestResponse, err := slackAPI.AuthTest()
	config.SlackBotID = authTestResponse.UserID

	// Initialize the actual data we need for the status page
	globalSites, globalUpdates, globalPinnedUpdates, err = buildStatusPage()
	if err != nil {
		log.Fatal(err)
	}
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
	app.GET("/health", health)

	_ = app.Run()
}
