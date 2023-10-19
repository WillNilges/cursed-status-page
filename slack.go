package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

const (
	// Callback ID
	CSPUpdateStatusPage = "csp_update_status_page"
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
						slackAPI.RemoveReaction(reaction, slack.ItemRef{
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
						slackAPI.AddReaction(reaction, slack.NewRefToMessage(
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
		}
	}()
	slackSocket.Run()
}

func eventResp() func(c *gin.Context) {
	return func(c *gin.Context) {
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.String(http.StatusInternalServerError, "error reading slack event payload: %s", err.Error())
			return
		}
		event, err := slackevents.ParseEvent(bodyBytes, slackevents.OptionNoVerifyToken())
		if err != nil {
			c.String(http.StatusInternalServerError, "error reading slack event payload: %s", err.Error())
			return
		}
		log.Printf("%s\n", event.Type)
		switch event.Type {
		case slackevents.URLVerification:
			ve, ok := event.Data.(*slackevents.EventsAPIURLVerificationEvent)
			if !ok {
				c.String(http.StatusBadRequest, "invalid url verification event payload sent from slack")
				return
			}
			c.JSON(http.StatusOK, &slackevents.ChallengeResponse{
				Challenge: ve.Challenge,
			})
		case slackevents.AppRateLimited:
			c.String(http.StatusOK, "ack")
		case slackevents.CallbackEvent:
			innerEvent := event.InnerEvent
			shouldUpdate := false
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
				slackAPI.RemoveReaction(reaction, slack.ItemRef{
					Channel:   config.SlackStatusChannelID,
					Timestamp: ev.Item.Timestamp,
				})
				shouldUpdate = true
			case *slackevents.ReactionAddedEvent:
				reaction := ev.Reaction
				botMentioned, err := isBotMentioned(ev.Item.Timestamp)
				if err != nil {
					c.String(http.StatusInternalServerError, err.Error())
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
				slackAPI.AddReaction(reaction, slack.NewRefToMessage(
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
			default:
				c.String(http.StatusBadRequest, "no handler for event of given type")
			}
			// Update our history
			if shouldUpdate {
				globalChannelHistory, err = getChannelHistory()
				if err != nil {
					c.String(http.StatusInternalServerError, err.Error())
				}
				globalUpdates, globalPinnedUpdates, err = buildStatusPage()
				if err != nil {
					c.String(http.StatusInternalServerError, err.Error())
				}
			}
		default:
			c.String(http.StatusBadRequest, "invalid event type sent from slack")
		}
	}
}

func interactionResp() func(c *gin.Context) {
	return func(c *gin.Context) {
		var payload slack.InteractionCallback
		err := json.Unmarshal([]byte(c.Request.FormValue("payload")), &payload)
		if err != nil {
			c.String(http.StatusInternalServerError, "error reading slack interaction payload: %s", err.Error())
			return
		}

		if payload.Type == "message_action" {
			if payload.CallbackID == CSPUpdateStatusPage {
				globalChannelHistory, err = getChannelHistory()
				if err != nil {
					c.String(http.StatusInternalServerError, err.Error())
				}
				globalUpdates, globalPinnedUpdates, err = buildStatusPage()
				if err != nil {
					c.String(http.StatusInternalServerError, err.Error())
				}
			}
		} else {
			c.String(http.StatusBadRequest, "invalid event type sent from slack")
		}
	}
}
