package main

import (
	"errors"
	"fmt"
	"html/template"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"github.com/tidwall/gjson"
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
	app.getChannelHistory()
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
		botID := fmt.Sprintf("<@%s>", config.SlackBotID)
		// Ignore messages that don't mention us. Also, ignore messages that
		// mention us but are empty!
		if !strings.Contains(message.Text, botID) || message.Text == botID {
			continue
		}

		msgUser, err := app.slackSocket.GetUserInfo(message.User)
		if err != nil {
			log.Println(err)
			return err
		}
		realName := msgUser.RealName
		var update StatusUpdate
		update.Text = strings.Replace(message.Text, botID, "", -1)
		update.HTML = template.HTML(parseSlackMrkdwnLinks(update.Text))
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

type CSPSlackEvtHandler struct {
	*CSPSlack
	evt socketmode.Event
}

func (h *CSPSlackEvtHandler) handleEventAPIEvent() {
	eventsAPIEvent, ok := h.evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		fmt.Printf("Ignored %+v\n", h.evt)
		return
	}
	fmt.Printf("Event received: %+v\n", eventsAPIEvent)

	h.slackSocket.Ack(*h.evt.Request)

	switch eventsAPIEvent.Type {
	case slackevents.CallbackEvent:
		innerEvent := eventsAPIEvent.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.PinAddedEvent:
			h.shouldUpdate = true
		case *slackevents.PinRemovedEvent:
			h.shouldUpdate = true
		case *slackevents.ReactionRemovedEvent:
			if ev.User == config.SlackBotID {
				return
			}
			reaction := ev.Reaction
			h.slackSocket.RemoveReaction(reaction, slack.ItemRef{
				Channel:   config.SlackStatusChannelID,
				Timestamp: ev.Item.Timestamp,
			})
			h.shouldUpdate = true
		case *slackevents.ReactionAddedEvent:
			h.handleReactionAddedEvent(ev)	
		case *slackevents.MessageEvent:
			h.handleMessageEvent(ev)
		default:
			log.Println("no handler for event of given type")
		}
	default:
	}
}

func (h *CSPSlackEvtHandler) handleReactionAddedEvent(ev *slackevents.ReactionAddedEvent) {
	reaction := ev.Reaction
	botMentioned, err := h.isBotMentioned(ev.Item.Timestamp)
	if err != nil {
		log.Println(err)
		return
	}
	if ev.User == config.SlackBotID || !isRelevantReaction(reaction) || (!botMentioned) {
		return
	}
	// If necessary, remove a conflicting reaction
	if isRelevantReaction(reaction) {
		h.clearReactions(
			ev.Item.Timestamp,
			[]string{
				config.StatusOKEmoji,
				config.StatusWarnEmoji,
				config.StatusErrorEmoji,
			},
		)
	}
	// Mirror the reaction on the message
	h.slackSocket.AddReaction(reaction, slack.NewRefToMessage(
		config.SlackStatusChannelID,
		ev.Item.Timestamp,
	))
	h.shouldUpdate = true
}

func (h *CSPSlackEvtHandler) handleMessageEvent(ev *slackevents.MessageEvent) {
	// If a message mentioning us gets added or deleted, then
	// do something
	log.Printf("Message type: %s\n", ev.SubType)

	// If the message was deleted, then update the page.
	// If LITERALLY ANYTHING ELSE happened, bail
	switch ev.SubType {
	case "": // continue
	case "message_deleted":
		h.shouldUpdate = true
		fallthrough
	default:
		return
	}

	// If the bot was mentioned in this message, then we should probably
	// re-build the page, and if not, we should bail.
	botID := fmt.Sprintf("<@%s>", config.SlackBotID)
	if strings.Contains(ev.Text, botID) {
		h.shouldUpdate = true
	} else {
		return
	}

	// HACK: If we're still here, it means we got mentioned, and should
	// do something about it. We do this instead of an AppMention because
	// there does not seem to be any way to not fire an AppMentionEvent
	// if a message is edited
	log.Printf("Got mentioned. Timestamp is: %s. ThreadTimestamp is: %s\n", ev.TimeStamp, ev.ThreadTimeStamp)

	channelName, err := h.resolveChannelName(config.SlackForwardChannelID)
	if err != nil {
		log.Printf("Could not resolve channel name: %s\n", err)
		return
	}
	blocks := CreateUpdateResponseMsg(channelName, ev.User)
	_, _, err = h.slackSocket.PostMessage(config.SlackStatusChannelID, slack.MsgOptionTS(ev.TimeStamp), slack.MsgOptionBlocks(blocks...))
	if err != nil {
		log.Printf("Error posting ephemeral message: %s", err)
	}
}

func (h *CSPSlackEvtHandler) handleInteractiveEvent() {
	callback, ok := h.evt.Data.(slack.InteractionCallback)
	if !ok {
		fmt.Printf("Ignored %+v\n", h.evt)
		return
	}

	var payload interface{}

	switch callback.Type {
	case slack.InteractionTypeBlockActions:
		// See https://api.slack.com/apis/connections/socket-implement#button
		// Check which button was pressed
		for _, action := range callback.ActionCallback.BlockActions {
			switch action.ActionID {
			case CSPSetOK, CSPSetWarn, CSPSetError, CSPCancel:
				log.Printf("Block Action Detected: %s\n", action.ActionID)
				itemRef := slack.ItemRef{
					Channel:   callback.Channel.ID,
					Timestamp: callback.Container.ThreadTs,
				}

				selected_options := gjson.Get(string(callback.RawState), "values.options.options.selected_options").Array()
				for i, opt := range selected_options {
					fmt.Println(i, opt)

					option := gjson.Get(opt.String(), "value").String()
					switch option {
					case CSPPin:
						log.Println("Will pin message")

						// Get the conversation history
						err := h.slackSocket.AddPin(callback.Channel.ID, itemRef)
						if err != nil {
							log.Println(err)
						}
					case CSPForward:
						log.Println("Will forward message")

						messageText, err := h.getSingleMessage(callback.Channel.ID, callback.Container.ThreadTs)
						if err != nil {
							log.Println(err)
							break
						}
						_, _, err = h.slackSocket.PostMessage(config.SlackForwardChannelID, slack.MsgOptionText(messageText.Text, false))
					}
				}

				// Clear any old reactions
				switch action.ActionID {
				case CSPSetOK, CSPSetWarn, CSPSetError:
					h.clearReactions(
						callback.Container.ThreadTs,
						[]string{
							config.StatusOKEmoji,
							config.StatusWarnEmoji,
							config.StatusErrorEmoji,
						},
					)
				}

				// Add the reaction we want
				switch action.ActionID {
				case CSPSetOK:
					err := h.slackSocket.AddReaction(config.StatusOKEmoji, itemRef)
					if err != nil {
						// Handle the error
						h.slackSocket.Debugf("Error adding reaction: %v", err)
					}
				case CSPSetWarn:
					err := h.slackSocket.AddReaction(config.StatusWarnEmoji, itemRef)
					if err != nil {
						// Handle the error
						h.slackSocket.Debugf("Error adding reaction: %v", err)
					}
				case CSPSetError:
					err := h.slackSocket.AddReaction(config.StatusErrorEmoji, itemRef)
					if err != nil {
						// Handle the error
						h.slackSocket.Debugf("Error adding reaction: %v", err)
					}
				case CSPCancel:
				}
				_, _, err := h.slackSocket.DeleteMessage(config.SlackStatusChannelID, callback.Container.MessageTs)
				if err != nil {
					log.Println(err)
				}

			}
		}
	case slack.InteractionTypeShortcut:
		log.Printf("Got shortcut: %s", callback.CallbackID)
		if callback.CallbackID == CSPUpdateStatusPage {
			h.shouldUpdate = true
		}
	default:
		log.Println("no handler for event of given type")
	}

	h.slackSocket.Ack(*h.evt.Request, payload)
}

// Utility functions

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
	log.Println("Fetching channel history...")
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

