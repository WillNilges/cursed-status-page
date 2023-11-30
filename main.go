package main

import (
	"flag"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

type Config struct {
	OrgName    string
	LogoURL    string
	FaviconURL string

	SlackTeamID          string
	SlackAccessToken     string
	SlackAppToken        string
	SlackStatusChannelID string
	SlackForwardChannelID string
	SlackBotID           string
	SlackTruncation      string

	StatusNeutralColor string
	StatusOKColor      string
	StatusOKEmoji      string
	StatusWarnColor    string
	StatusWarnEmoji    string
	StatusErrorColor   string
	StatusErrorEmoji   string

	NominalMessage string
	NominalSentBy  string
	HelpMessage    string
}

type StatusUpdate struct {
	Text       string
	SentBy     string
	TimeStamp  string
	BackgroundClass string
	IconFilename string
}

var config Config

var globalChannelHistory []slack.Message

var globalUpdates []StatusUpdate
var globalPinnedUpdates []StatusUpdate

var slackAPI *slack.Client
var slackSocket *socketmode.Client

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
	config.SlackAppToken = os.Getenv("CSP_SLACK_APP_TOKEN")
	config.SlackStatusChannelID = os.Getenv("CSP_SLACK_STATUS_CHANNEL")
	config.SlackForwardChannelID = os.Getenv("CSP_SLACK_FORWARD_CHANNEL")
	config.SlackTruncation = os.Getenv("CSP_SLACK_TRUNCATION")

	config.StatusNeutralColor = os.Getenv("CSP_CARD_NEUTRAL_COLOR")
	config.StatusOKColor = os.Getenv("CSP_CARD_OK_COLOR")
	config.StatusOKEmoji = os.Getenv("CSP_CARD_OK_EMOJI")
	config.StatusWarnColor = os.Getenv("CSP_CARD_WARN_COLOR")
	config.StatusWarnEmoji = os.Getenv("CSP_CARD_WARN_EMOJI")
	config.StatusErrorColor = os.Getenv("CSP_CARD_ERROR_COLOR")
	config.StatusErrorEmoji = os.Getenv("CSP_CARD_ERROR_EMOJI")

	config.NominalMessage = os.Getenv("CSP_NOMINAL_MESSAGE")
	config.NominalSentBy = os.Getenv("CSP_NOMINAL_SENT_BY")
	config.HelpMessage = os.Getenv("CSP_HELP_LINK")

	pinReminders := flag.Bool("send-reminders", false, "Check for pinned items and send a reminder if it's been longer than a day.")

	flag.Parse()

	slackAPI := slack.New(config.SlackAccessToken, slack.OptionAppLevelToken(config.SlackAppToken))
	slackSocket = socketmode.New(slackAPI,
		socketmode.OptionLog(log.New(os.Stdout, "socketmode: ", log.Lshortfile|log.LstdFlags)),
	)

	// Get the channel history
	globalChannelHistory, err = getChannelHistory()
	if err != nil {
		log.Fatal(err)
	}


	// Get some deets we'll need from the slack API
	authTestResponse, err := slackAPI.AuthTest()
	config.SlackBotID = authTestResponse.UserID

	// Send out reminders about pinned messages.
	if *pinReminders {
		sendReminders()
		os.Exit(0)
	} 

	// Initialize the actual data we need for the status page
	globalUpdates, globalPinnedUpdates, err = buildStatusPage()
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	go runSocket() // Start the Slack Socket

	app := gin.Default()
	app.LoadHTMLGlob("templates/*")
	app.Static("/static", "./static")

	app.GET("/", statusPage)
	app.GET("/health", health)

	_ = app.Run()
}
