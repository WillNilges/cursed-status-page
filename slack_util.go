package main

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

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

// Slack utility functions. Mostly just for data parsing. Don't actually reqire Slack
// client, but operate on Slack resources.

// Converts the timestamp from a message into a human-readable format.
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

// Function to build the message the bot sends in response to being pinged with
// a new status update.
func CreateUpdateResponseMsg(channelName string) (blocks []slack.Block) {
	blocks = []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, "I see you are posting a new message to the support page. What kind of alert is this? *Warning: this alert will go live immediately!*", false, false),
			nil,
			nil,
		),
		slack.NewInputBlock("options", slack.NewTextBlockObject(slack.PlainTextType, " ", false, false), nil,
			slack.NewCheckboxGroupsBlockElement(
				"options",
				slack.NewOptionBlockObject(
					CSPPin,
					slack.NewTextBlockObject(
						"plain_text",
						"Pin this message to the status page",
						false,
						false,
					),
					nil,
				),
				slack.NewOptionBlockObject(
					CSPForward,
					slack.NewTextBlockObject(
						"plain_text",
						fmt.Sprintf("Forward message to the #%s channel", channelName),
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
				slack.NewTextBlockObject("plain_text", "❌Close", true, false),
			),
		),
	}
	return blocks
}

func GetPinnedMessageStatus(reactions []slack.ItemReaction) string {
	for _, reaction := range reactions {
		// Only take action on our reactions
		if botReaction := stringInSlice(reaction.Users, config.SlackBotID); !botReaction {
			continue
		}

		// Use the first reaction sent by the bot that we find
		switch reaction.Name {
		case config.StatusOKEmoji:
			return config.StatusOKEmoji
		case config.StatusWarnEmoji:
			return config.StatusWarnEmoji
		case config.StatusErrorEmoji:
			return config.StatusErrorEmoji
		}
	}
	return ""
}
