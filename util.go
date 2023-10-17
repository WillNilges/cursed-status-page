package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

func stringInSlice(searchSlice []string, searchString string) bool {
	for _, s := range searchSlice {
		if s == searchString {
			return true
		}
	}
	return false
}

// Slack utility functions

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

func getChannelHistory() (conversation []slack.Message, err error) {
	log.Println("Fetching channel history...")
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

func isBotMentioned(timestamp string) (isMentioned bool, err error) {
	history, err := slackAPI.GetConversationHistory(
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

	return strings.Contains(history.Messages[0].Text, config.SlackBotID), nil
}

func clearReactions(timestamp string, focusReactions []string) error {
	ref := slack.ItemRef{
		Channel:   config.SlackStatusChannelID,
		Timestamp: timestamp,
	}
	reactions, err := slackAPI.GetReactions(ref, slack.NewGetReactionsParameters())
	if err != nil {
		return err
	}
	if focusReactions == nil {
		for _, itemReaction := range reactions {
			err := slackAPI.RemoveReaction(itemReaction.Name, ref)
			if err != nil && err.Error() != "no_reaction" {
				return err
			}
		}
	} else {
		// No, I am not proud of this at all.
		for _, itemReaction := range focusReactions {
			err := slackAPI.RemoveReaction(itemReaction, ref)
			if err != nil && err.Error() != "no_reaction" {
				return err
			}
		}
	}
	return nil
}

func isRelevantReaction(reaction string, status bool, pin bool) bool {
	switch reaction {
	case config.StatusOKEmoji, config.StatusWarnEmoji, config.StatusErrorEmoji:
		if status {
			return true
		}
	case config.PinEmoji, config.CurrentEmoji:
		if pin {
			return true
		}
	}
	return false
}