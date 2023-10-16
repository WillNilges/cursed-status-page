package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
)

type GrabCallbackIDs string
type GrabBlockActionIDs string

const (
	// Callback ID
	GrabInteractionAppendThreadTranscript = "append_thread_transcript"
	// Block Action IDs for that Callback ID
	GrabInteractionAppendThreadTranscriptConfirm = "append_thread_transcript_confirm"
	GrabInteractionAppendThreadTranscriptCancel  = "append_thread_transcript_cancel"

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
			switch ev := innerEvent.Data.(type) {
			case *slackevents.ReactionAddedEvent:
				reaction := ev.Reaction
				log.Println(reaction)
				// Mirror the reaction on the message
			case *slackevents.MessageEvent:
				statusHistory, err = getStatusHistory()
				if err != nil {
					c.String(http.StatusInternalServerError, err.Error())
				}
			case *slackevents.AppMentionEvent:
				statusHistory, err = getStatusHistory()
				if err != nil {
					c.String(http.StatusInternalServerError, err.Error())
				}
			default:
				c.String(http.StatusBadRequest, "no handler for event of given type")
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
				_, err = slackAPI.PostEphemeral(
					payload.Channel.ID,
					payload.User.ID,
					slack.MsgOptionTS(payload.Message.ThreadTimestamp),
					slack.MsgOptionText("Chom skz", false),
				)

			}

		}

		// TODO: Else get angery
	}
}

func getThreadConversation(api *slack.Client, channelID string, threadTs string) (conversation []slack.Message, err error) {
	// Get the conversation history
	params := slack.GetConversationRepliesParameters{
		ChannelID: channelID,
		Timestamp: threadTs,
	}
	conversation, _, _, err = api.GetConversationReplies(&params)
	if err != nil {
		return conversation, err
	}
	return conversation, nil
}

func getStatusHistory() (conversation []slack.Message, err error) {
	log.Println("Fetching Status Updates...")
	limit, _ := strconv.Atoi(config.SlackTruncation)
	params := slack.GetConversationHistoryParameters{
		ChannelID: config.SlackStatusChannelID,
		Oldest:    "0",   // Retrieve messages from the beginning of time
		Inclusive: true,  // Include the oldest message
		Limit:     limit, // Only get 100 messages
	}

	var history *slack.GetConversationHistoryResponse
	history, err = slackAPI.GetConversationHistory(&params)
	return history.Messages, err
}
/*
func removeAllReactions(timestamp string) error {
	reactions, err := slackAPI.GetReactions(slack.ItemRef{
		Channel: config.SlackStatusChannelID,
		Timestamp: timestamp,
	},
	slack.GetReactionsParameters{
		Full: false,
	})
	if err != nil {
		return err
	}

	for _, reaction := range reactions {
		log.Println("I bet this code isn't running")
		if err := slackAPI.RemoveReaction(reaction.Name, slack.ItemRef{
			Channel:   config.SlackStatusChannelID,
			Timestamp: timestamp,
		}); err != nil {
			log.Printf("Error removing reaction %s: %v\n", reaction.Name, err)
		}
	}

	return nil
}*/

func removeAllReactions(timestamp string) error {
	ref := slack.ItemRef{
		Channel:   config.SlackStatusChannelID,
		Timestamp: timestamp,
	}
	reactions, err := slackAPI.GetReactions(ref, slack.NewGetReactionsParameters())
	if err != nil {
		return err
	}
	for _, itemReaction := range reactions {
		log.Println(itemReaction)
		err := slackAPI.RemoveReaction(itemReaction.Name, ref)
		if err != nil {
			return err
		}
	}
	return nil
}
