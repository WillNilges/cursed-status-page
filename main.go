package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

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

	CurrentEmoji string

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

var config Config

var statusHistory []slack.Message

var globalUpdates []StatusUpdate
var globalPinnedUpdates []StatusUpdate
var globalCurrentStatus StatusUpdate

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

	config.CurrentEmoji = os.Getenv("CSP_CURRENT_EMOJI")

	config.NominalMessage = os.Getenv("CSP_NOMINAL_MESSAGE")
	config.NominalSentBy = os.Getenv("CSP_NOMINAL_SENT_BY")
	config.HelpMessage = os.Getenv("CSP_HELP_LINK")

	slackAPI = slack.New(config.SlackAccessToken)

	statusHistory, err = getChannelHistory()
	if err != nil {
		log.Fatal(err)
	}

	authTestResponse, err := slackAPI.AuthTest()
	config.SlackBotID = authTestResponse.UserID

	globalUpdates, globalPinnedUpdates, globalCurrentStatus, err = buildStatusPage()
	if err != nil {
		log.Fatal(err)
	}
}

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

func stringInSlice(searchSlice []string, searchString string) bool {
	for _, s := range searchSlice {
		if s == searchString {
			return true
		}
	}
	return false
}

func buildStatusPage() (updates []StatusUpdate, pinnedUpdates []StatusUpdate, currentStatus StatusUpdate, err error){
	log.Println("Building Status Page...")
	hasCurrentStatus := false
	for _, message := range statusHistory {
		teamID := fmt.Sprintf("<@%s>", config.SlackBotID)
		// Ignore messages that don't mention us. Also, ignore messages that
		// mention us but are empty!
		if !strings.Contains(message.Text, teamID) || message.Text == teamID {
			continue
		}
		msgUser, err := slackAPI.GetUserInfo(message.User)
		if err != nil {
			log.Println(err)
			//c.String(http.StatusInternalServerError, "error reading request body: %s", err.Error())
			return updates, pinnedUpdates, currentStatus, err
		}
		realName := msgUser.RealName
		var update StatusUpdate
		update.Text = strings.Replace(message.Text, teamID, "", -1)
		update.SentBy = realName
		update.TimeStamp = slackTSToHumanTime(message.Timestamp)
		update.Background = config.StatusNeutralColor

		willBeCurrentStatus := false
		shouldPin := false
		for _, reaction := range message.Reactions {
			// Only take action on our reactions
			if botReaction := stringInSlice(reaction.Users, config.SlackBotID); !botReaction {
				continue
			}
			// If we find a pin at all, then use it
			if reaction.Name == config.CurrentEmoji && hasCurrentStatus == false {
				willBeCurrentStatus = true
			} else if reaction.Name == config.PinEmoji {
				shouldPin = true
			}

			// Use the first reaction sent by the bot that we find
			if update.Background == config.StatusNeutralColor {
				switch reaction.Name {
				case config.StatusOKEmoji:
					update.Background = config.StatusOKColor
				case config.StatusWarnEmoji:
					update.Background = config.StatusWarnColor
				case config.StatusErrorEmoji:
					update.Background = config.StatusErrorColor
				}
			}
		}
		if willBeCurrentStatus {
			currentStatus = update
			hasCurrentStatus = true
		} else if shouldPin && len(pinnedUpdates) < config.PinLimit {
			pinnedUpdates = append(pinnedUpdates, update)
		} else {
			updates = append(updates, update)
		}
	}

	if !hasCurrentStatus {
		currentStatus = StatusUpdate{
			Text:       config.NominalMessage,
			SentBy:     config.NominalSentBy,
			TimeStamp:  "Now",
			Background: config.StatusOKColor,
		}
	}

	return updates, pinnedUpdates, currentStatus, nil
}

func statusPage(c *gin.Context) {
	c.HTML(
		http.StatusOK,
		"index.html",
		gin.H{
			"HelpMessage":    template.HTML(config.HelpMessage),
			"PinnedStatuses": globalPinnedUpdates,
			"CurrentStatus":  globalCurrentStatus,
			"StatusUpdates":  globalUpdates,
			"Org":            config.OrgName,
			"Logo":           config.LogoURL,
			"Favicon":        config.FaviconURL,
		},
	)
}

func health(c *gin.Context) {
	c.JSON(http.StatusOK, "cursed-status-page")
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
