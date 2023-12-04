package main

import (
	"flag"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

type Config struct {
	OrgName    string
	LogoURL    string
	FaviconURL string

	SlackTeamID           string
	SlackAccessToken      string
	SlackAppToken         string
	SlackStatusChannelID  string
	SlackForwardChannelID string
	SlackBotID            string
	SlackTruncation       string

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

// Useful global variables
var config Config

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

	if *pinReminders {
		csp, err := NewCSPSlack()
		if err != nil {
			log.Fatalf("Could not set up new CSPSlack service. %s", err)
		}
		err = csp.SendReminders()
		os.Exit(0)
	}
}

func main() {
	csp, err := NewCSPSlack()
	if err != nil {
		log.Fatalf("Could not set up new CSPSlack service. %s", err)
	}
	go csp.Run() // Start the Slack Socket

	app := gin.Default()
	app.LoadHTMLGlob("templates/*")
	app.Static("/static", "./static")

	app.GET("/", csp.page.statusPage)
	app.GET("/health", health)

	_ = app.Run()
}
