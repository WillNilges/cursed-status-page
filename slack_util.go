package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

// Slack utility functions. Mostly just for data parsing. Don't actually reqire Slack
// client, but operate on Slack resources.
func parseSlackMrkdwnLinks(message string) string {
	// Regular expression to match links with optional labels
	linkRegex := regexp.MustCompile(`<([^|>]+)\|([^>]+)>|<([^>]+)>`)

	// Replace links with HTML-formatted links
	result := linkRegex.ReplaceAllStringFunc(message, func(match string) string {
		// If the link has a label, use it as the anchor text, otherwise use the URL
		parts := strings.Split(match[1:len(match)-1], "|")
		if len(parts) == 2 {
			return fmt.Sprintf(`<a target="_blank" href="%s">%s</a>`, parts[0], parts[1])
		} else {
			return fmt.Sprintf(`<a target="_blank" href="%s">%s</a>`, parts[0], parts[0])
		}
	})

	return result
}

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
