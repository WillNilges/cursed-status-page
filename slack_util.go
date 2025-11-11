package main

import (
	"fmt"
	"html/template"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gomarkdown/markdown"
	"github.com/microcosm-cc/bluemonday"
	"github.com/slack-go/slack"
)

func MrkdwnToHTML(message string) (formattedHtml template.HTML) {
	md := mrkdwnToMarkdown(message)
	maybeUnsafeHTML := markdown.ToHTML([]byte(md), nil, nil)
	html := bluemonday.UGCPolicy().SanitizeBytes(maybeUnsafeHTML)
	return template.HTML(html)
}

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

// REALLY SHITTY parser from ChatGPT. I spent some time fucking around with the
// Blocks and have concluded that writing a parser for that shit is a whole other
// project in and of itself. Maybe someday. For now, my shit will probably be
// vulnerable to regex-based attacks.
func mrkdwnToMarkdown(input string) string {
	// Handle bold text
	boldRegex := regexp.MustCompile(`\*(.*?)\*`)
	input = boldRegex.ReplaceAllString(input, "**$1**")

	// Handle italic text
	italicRegex := regexp.MustCompile(`_(.*?)_`)
	input = italicRegex.ReplaceAllString(input, "*$1*")

	// Handle strikethrough text
	strikeRegex := regexp.MustCompile(`~(.*?)~`)
	input = strikeRegex.ReplaceAllString(input, "~~$1~~")

	// Handle code blocks
	codeRegex := regexp.MustCompile("`([^`]+)`")
	input = codeRegex.ReplaceAllString(input, "`$1`")

	// Handle links with labels
	linkWithLabelRegex := regexp.MustCompile(`<([^|]+)\|([^>]+)>`)
	input = linkWithLabelRegex.ReplaceAllString(input, "[$2]($1)")

	// Handle links without labels
	linkWithoutLabelRegex := regexp.MustCompile(`<([^>]+)>`)
	input = linkWithoutLabelRegex.ReplaceAllString(input, "[$1]($1)")

	// Slack, for some insane reason, gives us messages with < formatted as &lt;,
	// so we need to correct that.
	// TODO: There are probably others but I'd have to write a test suite for
	// that. There has got to be a better way to do this.
	lessThanRegex := regexp.MustCompile(`&lt;`)
	input = lessThanRegex.ReplaceAllString(input, "<")
	greaterThanRegex := regexp.MustCompile(`&gt;`)
	input = greaterThanRegex.ReplaceAllString(input, ">")

	return input
}

func slackTSToTime(slackTimestamp string) (slackTime time.Time) {
	// Convert the Slack timestamp to a Unix timestamp (float64)
	slackUnixTimestamp, err := strconv.ParseFloat(strings.Split(slackTimestamp, ".")[0], 64)
	if err != nil {
		fmt.Println("Error parsing Slack timestamp:", err)
		return
	}

	// Create a time.Time object from the Unix timestamp (assuming UTC time zone)
	slackTime = time.Unix(int64(slackUnixTimestamp), 0)
	return slackTime
}

// Converts the timestamp from a message into a human-readable format.
func slackTSToHumanTime(slackTimestamp string) (hrt string) {
	slackTime := slackTSToTime(slackTimestamp)

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
func CreateUpdateResponseMsg(channelName string, user string) (blocks []slack.Block) {
	blocks = []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("<@%s> I see you have posted a new message to the support page. What kind of alert is this? *Warning: this alert is live immediately!*", user), false, false),
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

// Ignore messages that don't mention us. Also, ignore messages that
// mention us but are empty!
func botActionablyMentioned(message string) bool {
	botID := fmt.Sprintf("<@%s>", config.SlackBotID)
	if !strings.Contains(message, botID) || message == botID {
		return false
	}
	return true
}
