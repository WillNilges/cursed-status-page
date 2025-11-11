package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

const (
	// Callback ID
	CSPUpdateStatusPage = "csp_update_status_page"
	CSPSetOK            = "csp_set_ok"
	CSPSetWarn          = "csp_set_warn"
	CSPSetError         = "csp_set_errteamID"
	CSPCancel           = "csp_cancel"

	CSPPin = "pin"

	CSPForward = "forward"
)

type CSPSlack struct {
	slackAPI    *slack.Client
	slackSocket *socketmode.Client

	channelHistory []slack.Message

	shouldUpdate bool

	page CSPPage
}

func NewCSPSlack() (app CSPSlack, err error) {
	app.slackAPI = slack.New(config.SlackAccessToken, slack.OptionAppLevelToken(config.SlackAppToken))
	app.slackSocket = socketmode.New(app.slackAPI,
		socketmode.OptionLog(log.New(os.Stdout, "socketmode: ", log.Lshortfile|log.LstdFlags)),
	)

	// Get the channel history
	err = app.getChannelHistory()
	if err != nil {
		log.Fatal(err)
	}

	// Get some deets we'll need from the slack API
	authTestResponse, err := app.slackAPI.AuthTest()
	config.SlackBotID = authTestResponse.UserID

	// Initialize the actual data we need for the status page
	err = app.BuildStatusPage()
	if err != nil {
		log.Fatal(err)
	}
	return app, nil
}

// Nuke the old slices and re-build them
func (app *CSPSlack) BuildStatusPage() (err error) {
	log.Println("Building Status Page...")
	app.page.updates = make([]StatusUpdate, 0)
	app.page.pinnedUpdates = make([]StatusUpdate, 0)
	for _, message := range app.channelHistory {
		// Ignore messages that don't mention us. Also, ignore messages that
		// mention us but are empty!
		if !botActionablyMentioned(message.Text) {
			continue
		}

		msgUser, err := app.slackSocket.GetUserInfo(message.User)
		if err != nil {
			log.Println(err)
			return err
		}
		realName := msgUser.RealName
		var update StatusUpdate

		log.Printf("Received message: %s\n", message.Text)

		// Disgusting dependency chain to parse Mrkdwn to HTML
		botID := fmt.Sprintf("<@%s>", config.SlackBotID)
		noBots := strings.Replace(message.Text, botID, "", -1)
		humanifiedChannels, err := app.slackChannelLinksToMarkdown(noBots)
		if err != nil {
			return err
		}
		update.HTML = MrkdwnToHTML(humanifiedChannels)

		update.SentBy = realName
		update.TimeStamp = slackTSToHumanTime(message.Timestamp)
		update.BackgroundClass = ""
		update.IconFilename = ""

		for _, reaction := range message.Reactions {
			// Only take action on our reactions
			if botReaction := stringInSlice(reaction.Users, config.SlackBotID); !botReaction {
				continue
			}

			// Use the first reaction sent by the bot that we find
			switch reaction.Name {
			case config.StatusOKEmoji:
				update.BackgroundClass = "list-group-item-success"
				update.IconFilename = "checkmark.svg"
			case config.StatusWarnEmoji:
				update.BackgroundClass = "list-group-item-warning"
				update.IconFilename = "warning.svg"
			case config.StatusErrorEmoji:
				update.BackgroundClass = "list-group-item-danger"
				update.IconFilename = "error.svg"
			}

		}
		if len(message.PinnedTo) > 0 {
			app.page.pinnedUpdates = append(app.page.pinnedUpdates, update)
		} else {
			app.page.updates = append(app.page.updates, update)
		}

	}

	return nil
}

// Pass-Thru the interface to the Page object
func (app *CSPSlack) StatusPage(gin *gin.Context) {
	app.page.statusPage(gin)
}

func (app *CSPSlack) SendReminders(now bool) error {
	fmt.Println("Sending unpin reminders...")
	var pinnedMessageLinks []ReminderInfo
	for _, message := range app.channelHistory {
		// Don't send reminders for messages that don't mention the bot.
		// That way, we can still pin messages.
		if !botActionablyMentioned(message.Text) {
			continue
		}
		if len(message.PinnedTo) > 0 {
			ts := slackTSToHumanTime(message.Timestamp)
			status := GetPinnedMessageStatus(message.Reactions)

			// Don't bother if the message hasn't been up longer than a day
			t, err := time.Parse("2006-01-02 15:04:05 MST", ts)
			if err == nil {
				if time.Since(t) < 24*time.Hour && now == false {
					fmt.Println("Message not pinned for long enough. Ignoring.")
					continue
				}
			}

			// Grab permalink to send final reminder message.
			permalink, err := app.slackSocket.GetPermalink(&slack.PermalinkParameters{
				Channel: config.SlackStatusChannelID,
				Ts:      message.Timestamp,
			})
			if err != nil {
				return err
			}
			pinnedMessageLinks = append(pinnedMessageLinks, ReminderInfo{message.User, permalink, ts, status})
			fmt.Println("Found message.")
		}
	}

	if len(pinnedMessageLinks) == 0 {
		fmt.Println("No messages pinned.")
		return nil
	}

	// Send summary message
	summaryMessage := fmt.Sprintln("Hello, Admins.\nThe following messages are currently pinned.")
	for _, m := range pinnedMessageLinks {
		var parsedStatus string
		if m.status == "" {
			parsedStatus = "•"
		} else {
			parsedStatus = fmt.Sprintf(":%s:", m.status)
		}
		summaryMessage += fmt.Sprintf("%s <@%s> <%s|Since %s>\n\n", parsedStatus, m.userID, m.link, m.ts)
	}

	summaryMessage += fmt.Sprintf("It might be time to unpin them if they are no longer relevant.")

	_, _, err := app.slackSocket.PostMessage(
		config.SlackStatusChannelID,
		slack.MsgOptionText(summaryMessage, false),
	)
	if err != nil {
		return err
	}

	fmt.Println("success.")
	return nil
}

func (app *CSPSlack) Run() {
	go func() {
		for evt := range app.slackSocket.Events {
			e := CSPSlackEvtHandler{app, evt}
			log.Println("Got event:", evt.Type)
			switch evt.Type {
			case socketmode.EventTypeConnecting:
				fmt.Println("Connecting to Slack with Socket Mode...")
			case socketmode.EventTypeConnectionError:
				fmt.Println("Connection failed. Retrying later...")
			case socketmode.EventTypeConnected:
				fmt.Println("Connected to Slack with Socket Mode.")
			case socketmode.EventTypeEventsAPI:
				e.handleEventAPIEvent()
			case socketmode.EventTypeInteractive:
				e.handleInteractiveEvent()
			}

			// If necessary, sync our cached Slack messages
			// and re-build the page history
			if app.shouldUpdate {
				var err error
				err = app.getChannelHistory()
				if err != nil {
					log.Println(err.Error())
				}
				err = app.BuildStatusPage()
				if err != nil {
					log.Println(err.Error())
				}
				app.shouldUpdate = false
			}
		}
	}()
	app.slackSocket.Run()
}

// Utility functions

func (app *CSPSlack) slackChannelLinksToMarkdown(input string) (string, error) {
	linkWithoutLabelRegex := regexp.MustCompile(`<#(C[A-Z0-9]+)\|>`)

	teamInfo, err := app.slackAPI.GetTeamInfo()
	if err != nil {
		return "", err
	}
	domain := teamInfo.Domain

	output := linkWithoutLabelRegex.ReplaceAllStringFunc(input, func(channelID string) string {
		channelID = channelID[2 : len(channelID)-2] // Hack to trim the <# and |> off the channel
		channelName, err := app.resolveChannelName(channelID)
		if err != nil {
			log.Println("Error: Did not get channel name for channel ", channelID)
			channelName = "unknown" // FIXME: (wdn) - This is a hack.
		}

		log.Println(channelName)

		return fmt.Sprintf("[#%s](https://%s.slack.com/archives/%s)", channelName, domain, channelID)
	})

	return output, nil
}

// ResolveChannelName retrieves the human-readable channel name from the channel ID.
func (app *CSPSlack) resolveChannelName(channelID string) (string, error) {
	info, err := app.slackSocket.GetConversationInfo(&slack.GetConversationInfoInput{
		ChannelID:         channelID,
		IncludeLocale:     false,
		IncludeNumMembers: false,
	})
	if err != nil {
		return "", err
	}
	return info.Name, nil
}

func (app *CSPSlack) getThreadConversation(channelID string, threadTs string) (conversation []slack.Message, err error) {
	// Get the conversation history
	params := slack.GetConversationRepliesParameters{
		ChannelID: channelID,
		Timestamp: threadTs,
	}
	conversation, _, _, err = app.slackAPI.GetConversationReplies(&params)
	if err != nil {
		return conversation, err
	}
	return conversation, nil
}

func (app *CSPSlack) getChannelHistory() (err error) {
	log.Println("Fetching channel history from: ", config.SlackStatusChannelID)
	limit, _ := strconv.Atoi(config.SlackTruncation)
	params := slack.GetConversationHistoryParameters{
		ChannelID: config.SlackStatusChannelID,
		Oldest:    "0",   // Retrieve messages from the beginning of time
		Inclusive: true,  // Include the oldest message
		Limit:     limit, // Only get 100 messages
	}

	var history *slack.GetConversationHistoryResponse
	history, err = app.slackSocket.GetConversationHistory(&params)
	app.channelHistory = history.Messages
	return err
}

func (app *CSPSlack) getSingleMessage(channelID string, oldest string) (message slack.Message, err error) {
	log.Println("Fetching channel history...")
	params := slack.GetConversationHistoryParameters{
		ChannelID: channelID,
		Oldest:    oldest,
		Inclusive: true,
		Limit:     1,
	}

	var history *slack.GetConversationHistoryResponse
	history, err = app.slackSocket.GetConversationHistory(&params)
	if err != nil {
		return message, err
	}
	if len(history.Messages) == 0 {
		return message, errors.New("No messages retrieved.")
	}
	return history.Messages[0], err
}

func (app *CSPSlack) isBotMentioned(timestamp string) (isMentioned bool, err error) {
	history, err := app.slackSocket.GetConversationHistory(
		&slack.GetConversationHistoryParameters{
			ChannelID: config.SlackStatusChannelID,
			Inclusive: true,
			Latest:    timestamp,
			Oldest:    timestamp,
			Limit:     1,
		},
	)
	if err != nil {
		return false, err
	}
	if len(history.Messages) > 0 {
		return strings.Contains(history.Messages[0].Text, config.SlackBotID), nil
	}
	return false, err
}

func (app *CSPSlack) clearReactions(timestamp string, focusReactions []string) error {
	ref := slack.ItemRef{
		Channel:   config.SlackStatusChannelID,
		Timestamp: timestamp,
	}
	reactions, err := app.slackSocket.GetReactions(ref, slack.NewGetReactionsParameters())
	if err != nil {
		return err
	}
	if focusReactions == nil {
		for _, itemReaction := range reactions {
			err := app.slackSocket.RemoveReaction(itemReaction.Name, ref)
			if err != nil && err.Error() != "no_reaction" {
				return err
			}
		}
	} else {
		// No, I am not proud of this at all.
		for _, itemReaction := range focusReactions {
			err := app.slackSocket.RemoveReaction(itemReaction, ref)
			if err != nil && err.Error() != "no_reaction" {
				return err
			}
		}
	}
	return nil
}
