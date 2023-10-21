package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/gin-gonic/gin"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

const (
	// Callback ID
	CSPUpdateStatusPage = "csp_update_status_page"
	CSPSetOK            = "csp_set_ok"
	CSPSetWarn          = "csp_set_warn"
	CSPSetError         = "csp_set_errteamID"
	CSPCancel           = "csp_cancel"
)

func signatureVerification(c *gin.Context) {
	verifier, err := slack.NewSecretsVerifier(c.Request.Header, os.Getenv("CSP_SLACK_SIGNING_SECRET"))
	if err != nil {
		c.String(http.StatusBadRequest, "error initializing signature verifier: %s", err.Error())
		return
	}
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.String(http.StatusInternalServerError, "error reading request body: %s", err.Error())
		return
	}
	bodyBytesCopy := make([]byte, len(bodyBytes))
	copy(bodyBytesCopy, bodyBytes)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytesCopy))
	if _, err = verifier.Write(bodyBytes); err != nil {
		c.String(http.StatusInternalServerError, "error writing request body bytes for verification: %s", err.Error())
		return
	}
	if err = verifier.Ensure(); err != nil {
		c.String(http.StatusUnauthorized, "error verifying slack signature: %s", err.Error())
		return
	}
	c.Next()
}

func installResp() func(c *gin.Context) {
	return func(c *gin.Context) {
		_, errExists := c.GetQuery("error")
		if errExists {
			c.String(http.StatusOK, "error installing app")
			return
		}
		code, codeExists := c.GetQuery("code")
		if !codeExists {
			c.String(http.StatusBadRequest, "missing mandatory 'code' query parameter")
			return
		}
		resp, err := slack.GetOAuthV2Response(http.DefaultClient,
			os.Getenv("CSP_SLACK_CLIENT_ID"),
			os.Getenv("CSP_SLACK_CLIENT_SECRET"),
			code,
			"")
		if err != nil {
			c.String(http.StatusInternalServerError, "error exchanging temporary code for access token: %s", err.Error())
			return
		}

		c.Redirect(http.StatusFound, fmt.Sprintf("slack://app?team=%s&id=%s&tab=about", resp.Team.ID, resp.AppID))
	}
}

func runSocket() {
	go func() {
		for evt := range slackSocket.Events {
			switch evt.Type {
			case socketmode.EventTypeConnecting:
				fmt.Println("Connecting to Slack with Socket Mode...")
			case socketmode.EventTypeConnectionError:
				fmt.Println("Connection failed. Retrying later...")
			case socketmode.EventTypeConnected:
				fmt.Println("Connected to Slack with Socket Mode.")
			case socketmode.EventTypeEventsAPI:
				eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					fmt.Printf("Ignored %+v\n", evt)

					continue
				}
				fmt.Printf("Event received: %+v\n", eventsAPIEvent)

				slackSocket.Ack(*evt.Request)

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

						// Create the message blocks
						blocks := []slack.Block{
							slack.NewSectionBlock(
								slack.NewTextBlockObject(slack.MarkdownType, "I see you are posting a new message to the support page. What kind of alert is this? *Warning: this alert will go live immediately!*", false, false),
								nil,
								nil,
							),
							slack.NewInputBlock("pin", slack.NewTextBlockObject(slack.PlainTextType, " ", false, false), nil,
								slack.NewCheckboxGroupsBlockElement(
									"pin", slack.NewOptionBlockObject(
										"pin",
										slack.NewTextBlockObject(
											"plain_text",
											"Pin this message to the status page",
											false,
											false,
										),
										nil,
									),
								),
							),
							slack.NewActionBlock(
								"",
								slack.NewButtonBlockElement(
									CSPSetError,
									CSPSetError,
									slack.NewTextBlockObject("plain_text", "🔥 Critical", true, false),
								),
								slack.NewButtonBlockElement(
									CSPSetWarn,
									CSPSetWarn,
									slack.NewTextBlockObject("plain_text", "⚠️ Warning", true, false),
								),
								slack.NewButtonBlockElement(
									CSPSetOK,
									CSPSetOK,
									slack.NewTextBlockObject("plain_text", "✅ OK/Info", true, false),
								),
								slack.NewButtonBlockElement(
									CSPCancel,
									CSPCancel,
									slack.NewTextBlockObject("plain_text", "🗑️ Cancel", true, false),
								),
							),
						}

						//FIXME (willnilges): Seems like slack has some kind of limitation with being unable to post ephemeral messages to threads and then
						// broadcast them to channels. So for now this is going to be non-ephemeral.

						// Post the ephemeral message
						//_, _, err := slackSocket.PostMessage(config.SlackStatusChannelID, slack.MsgOptionTS(ev.TimeStamp), slack.MsgOptionText("Hello!", false))
						//_, err = slackSocket.PostEphemeral(config.SlackStatusChannelID, ev.User, slack.MsgOptionTS(ev.TimeStamp), slack.MsgOptionBroadcast(), slack.MsgOptionBlocks(blocks...))
						_, _, err := slackSocket.PostMessage(config.SlackStatusChannelID, slack.MsgOptionTS(ev.TimeStamp), slack.MsgOptionBroadcast(), slack.MsgOptionBlocks(blocks...))
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
			case socketmode.EventTypeInteractive:
				callback, ok := evt.Data.(slack.InteractionCallback)
				if !ok {
					fmt.Printf("Ignored %+v\n", evt)
					continue
				}

				fmt.Printf("Interaction received: %+v\n", callback)

				var payload interface{}

				switch callback.Type {
				case slack.InteractionTypeBlockActions:
					// See https://api.slack.com/apis/connections/socket-implement#button
					// Check which button was pressed
					for _, action := range callback.ActionCallback.BlockActions {
						switch action.ActionID {
						case CSPSetOK, CSPSetWarn, CSPSetError:
							log.Printf("Block Action Detected: %s\n", action.ActionID)
							itemRef := slack.ItemRef{
								Channel:   callback.Channel.ID,
								Timestamp: callback.Container.ThreadTs,
							}

							// Check if the message should be pinned
							maybePin := gjson.Get(string(callback.RawState), "values.pin.pin.selected_options.0.value").String()
							if maybePin == "pin" {
								log.Println("Will pin message!")

								// Get the conversation history
								err := slackSocket.AddPin(callback.Channel.ID, itemRef)
								if err != nil {
									log.Println(err)
								}
							}

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
							}

							slackSocket.DeleteMessage(config.SlackStatusChannelID, callback.Container.MessageTs)
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

				slackSocket.Ack(*evt.Request, payload)

			}
		}
	}()
	slackSocket.Run()
}
