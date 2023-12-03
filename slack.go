package main

import (
	"fmt"
	"log"
	"strings"

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

func runSocket() {
	go func() {
		for evt := range slackSocket.Events {
			e := CSPSlackEvtHandler{evt}
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
		}
	}()
	slackSocket.Run()
}

type CSPSlackEvtHandler struct {
	evt socketmode.Event
}

func (h *CSPSlackEvtHandler) handleEventAPIEvent() {
	eventsAPIEvent, ok := h.evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		fmt.Printf("Ignored %+v\n", h.evt)
		return
	}
	fmt.Printf("Event received: %+v\n", eventsAPIEvent)

	slackSocket.Ack(*h.evt.Request)

	switch eventsAPIEvent.Type {
	case slackevents.CallbackEvent:
		shouldUpdate := false
		innerEvent := eventsAPIEvent.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.PinAddedEvent:
			shouldUpdate = true
		case *slackevents.PinRemovedEvent:
			shouldUpdate = true
		case *slackevents.ReactionRemovedEvent:
			if ev.User == config.SlackBotID {
				break
			}
			reaction := ev.Reaction
			slackSocket.RemoveReaction(reaction, slack.ItemRef{
				Channel:   config.SlackStatusChannelID,
				Timestamp: ev.Item.Timestamp,
			})
			shouldUpdate = true
		case *slackevents.ReactionAddedEvent:
			reaction := ev.Reaction
			botMentioned, err := isBotMentioned(ev.Item.Timestamp)
			if err != nil {
				log.Println(err)
				break
			}
			if ev.User == config.SlackBotID || !isRelevantReaction(reaction) || (!botMentioned) {
				break
			}
			// If necessary, remove a conflicting reaction
			if isRelevantReaction(reaction) {
				clearReactions(
					ev.Item.Timestamp,
					[]string{
						config.StatusOKEmoji,
						config.StatusWarnEmoji,
						config.StatusErrorEmoji,
					},
				)
			}
			// Mirror the reaction on the message
			slackSocket.AddReaction(reaction, slack.NewRefToMessage(
				config.SlackStatusChannelID,
				ev.Item.Timestamp,
			))
			shouldUpdate = true
		case *slackevents.MessageEvent:
			// If a message mentioning us gets added or deleted, then
			// do something
			log.Println(ev.SubType)
			// Check if a new message got posted to the site thread
			if (ev.Message != nil && strings.Contains(ev.Message.Text, config.SlackBotID)) || ev.SubType == "message_deleted" {
				shouldUpdate = true
			}
		case *slackevents.AppMentionEvent:
			shouldUpdate = true

			log.Printf("Got mentioned. Timestamp is: %s. ThreadTimestamp is: %s\n", ev.TimeStamp, ev.ThreadTimeStamp)

			channelName, err := ResolveChannelName(config.SlackForwardChannelID)
			if err != nil {
				log.Printf("Could not resolve channel name: %s\n", err)
				break
			}
			blocks := createUpdateResponseMsg(channelName) 
			//FIXME (willnilges): Seems like slack has some kind of limitation with being unable to post ephemeral messages to threads and then
			// broadcast them to channels. So for now this is going to be non-ephemeral.

			// Post the ephemeral message
			//_, _, err := slackSocket.PostMessage(config.SlackStatusChannelID, slack.MsgOptionTS(ev.TimeStamp), slack.MsgOptionText("Hello!", false))
			//_, err = slackSocket.PostEphemeral(config.SlackStatusChannelID, ev.User, slack.MsgOptionTS(ev.TimeStamp), slack.MsgOptionBroadcast(), slack.MsgOptionBlocks(blocks...))
			_, _, err = slackSocket.PostMessage(config.SlackStatusChannelID, slack.MsgOptionTS(ev.TimeStamp), slack.MsgOptionBroadcast(), slack.MsgOptionBlocks(blocks...))
			if err != nil {
				log.Printf("Error posting ephemeral message: %s", err)
			}

		default:
			log.Println("no handler for event of given type")
		}
		// Update our history
		if shouldUpdate {
			var err error
			globalChannelHistory, err = getChannelHistory()
			if err != nil {
				log.Println(err.Error())
			}
			globalUpdates, globalPinnedUpdates, err = buildStatusPage()
			if err != nil {
				log.Println(err.Error())
			}
		}
	default:
		slackSocket.Debugf("unsupported Events API event received")
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
						err := slackSocket.AddPin(callback.Channel.ID, itemRef)
						if err != nil {
							log.Println(err)
						}
					case CSPForward:
						log.Println("Will forward message")

						messageText, err := getSingleMessage(callback.Channel.ID, callback.Container.ThreadTs)
						if err != nil {
							log.Println(err)
							break
						}

						_, _, err = slackSocket.PostMessage(config.SlackForwardChannelID, slack.MsgOptionText(messageText.Text, false))

					}

				}

				// Clear any old reactions
				switch action.ActionID {
				case CSPSetOK, CSPSetWarn, CSPSetError:
					clearReactions(
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
					err := slackSocket.AddReaction(config.StatusOKEmoji, itemRef)
					if err != nil {
						// Handle the error
						slackSocket.Debugf("Error adding reaction: %v", err)
					}
				case CSPSetWarn:
					err := slackSocket.AddReaction(config.StatusWarnEmoji, itemRef)
					if err != nil {
						// Handle the error
						slackSocket.Debugf("Error adding reaction: %v", err)
					}
				case CSPSetError:
					err := slackSocket.AddReaction(config.StatusErrorEmoji, itemRef)
					if err != nil {
						// Handle the error
						slackSocket.Debugf("Error adding reaction: %v", err)
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
				_, _, err := slackSocket.DeleteMessage(config.SlackStatusChannelID, callback.Container.MessageTs)
				if err != nil {
					log.Println(err)
				}

			}
		}

	case slack.InteractionTypeShortcut:
		log.Printf("Got shortcut: %s", callback.CallbackID)
		if callback.CallbackID == CSPUpdateStatusPage {
			var err error
			globalChannelHistory, err = getChannelHistory()
			if err != nil {
				log.Println(err)
			}
			globalUpdates, globalPinnedUpdates, err = buildStatusPage()
			if err != nil {
				log.Println(err)
			}
		}
	case slack.InteractionTypeViewSubmission:
		// See https://api.slack.com/apis/connections/socket-implement#modal
	case slack.InteractionTypeDialogSubmission:
	default:

	}

	slackSocket.Ack(*h.evt.Request, payload)
}
