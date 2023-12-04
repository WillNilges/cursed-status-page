package main

import (
	"fmt"
	"log"
	"os"
	"strings"

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
	slackAPI *slack.Client
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

func (app *CSPSlack) StatusPage(gin *gin.Context) {
	app.page.statusPage(gin)
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
				break
			}
			reaction := ev.Reaction
			h.slackSocket.RemoveReaction(reaction, slack.ItemRef{
				Channel:   config.SlackStatusChannelID,
				Timestamp: ev.Item.Timestamp,
			})
			h.shouldUpdate = true
		case *slackevents.ReactionAddedEvent:
			reaction := ev.Reaction
			botMentioned, err := h.isBotMentioned(ev.Item.Timestamp)
			if err != nil {
				log.Println(err)
				break
			}
			if ev.User == config.SlackBotID || !isRelevantReaction(reaction) || (!botMentioned) {
				break
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
		case *slackevents.MessageEvent:
			// If a message mentioning us gets added or deleted, then
			// do something
			log.Println(ev.SubType)
			// Check if a new message got posted to the site thread
			if (ev.Message != nil && strings.Contains(ev.Message.Text, config.SlackBotID)) || ev.SubType == "message_deleted" {
				h.shouldUpdate = true
			}
		case *slackevents.AppMentionEvent:
			h.shouldUpdate = true

			log.Printf("Got mentioned. Timestamp is: %s. ThreadTimestamp is: %s\n", ev.TimeStamp, ev.ThreadTimeStamp)

			channelName, err := h.resolveChannelName(config.SlackForwardChannelID)
			if err != nil {
				log.Printf("Could not resolve channel name: %s\n", err)
				break
			}
			blocks := CreateUpdateResponseMsg(channelName) 
			//FIXME (willnilges): Seems like slack has some kind of limitation with being unable to post ephemeral messages to threads and then
			// broadcast them to channels. So for now this is going to be non-ephemeral.

			// Post the ephemeral message
			//_, _, err := slackSocket.PostMessage(config.SlackStatusChannelID, slack.MsgOptionTS(ev.TimeStamp), slack.MsgOptionText("Hello!", false))
			//_, err = slackSocket.PostEphemeral(config.SlackStatusChannelID, ev.User, slack.MsgOptionTS(ev.TimeStamp), slack.MsgOptionBroadcast(), slack.MsgOptionBlocks(blocks...))
			_, _, err = h.slackSocket.PostMessage(config.SlackStatusChannelID, slack.MsgOptionTS(ev.TimeStamp), slack.MsgOptionBroadcast(), slack.MsgOptionBlocks(blocks...))
			if err != nil {
				log.Printf("Error posting ephemeral message: %s", err)
			}

		default:
			log.Println("no handler for event of given type")
		}
	default:
		h.slackSocket.Debugf("unsupported Events API event received")
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
					// FIXME (willnilges): Seems like Slack won't let the bot delete a message without an admin account
					/*
						_, _, err := slackSocket.DeleteMessage(config.SlackStatusChannelID, callback.Container.ThreadTs)
						if err != nil {
							log.Println(err)

							_, _, err := slackSocket.PostMessage(config.SlackStatusChannelID, slack.MsgOptionTS(callback.Container.ThreadTs), slack.MsgOptionBroadcast(), slack.MsgOptionText("OK. Please remember to delete your message! I can't do it for you :(", false))
							if err != nil {
								log.Printf("Error posting ephemeral message: %s", err)
							}
						}*/
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

