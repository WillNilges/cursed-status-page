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
				if ev.User == config.SlackBotID || !isRelevantReaction(reaction, true, true) || !botMentioned {
					break
				}
				// If necessary, remove a conflicting reaction
				if isRelevantReaction(reaction, true, false) {
					clearReactions(
						ev.Item.Timestamp,
						[]string{
							config.StatusOKEmoji,
							config.StatusWarnEmoji,
							config.StatusErrorEmoji,
						},
					)
				} else if isRelevantReaction(reaction, false, true) {
					clearReactions(
						ev.Item.Timestamp,
						[]string{
							config.PinEmoji,
							config.CurrentEmoji,
						},
					)
				}
				// Mirror the reaction on the message
				slackAPI.AddReaction(reaction, slack.ItemRef{
					Channel:   config.SlackStatusChannelID,
					Timestamp: ev.Item.Timestamp,
				})
				shouldUpdate = true
			case *slackevents.MessageEvent:
				// If a message mentioning us gets added or deleted, then
				// do something
				log.Println(ev.SubType)
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
				globalUpdates, globalPinnedUpdates, globalCurrentStatus, err = buildStatusPage()
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
				globalUpdates, globalPinnedUpdates, globalCurrentStatus, err = buildStatusPage()
				if err != nil {
					c.String(http.StatusInternalServerError, err.Error())
				}
			}
		} else {
			c.String(http.StatusBadRequest, "invalid event type sent from slack")
		}
	}
}
