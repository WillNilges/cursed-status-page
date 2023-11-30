package main

import (
	"fmt"
	"github.com/slack-go/slack"
)

type ReminderInfo struct {
	link string
	ts string
	status string
}

func getPinnedMessageStatus(reactions []slack.ItemReaction) string {
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

func sendReminders() error {
	fmt.Println("Sending unpin reminders...")
	var pinnedMessageLinks []ReminderInfo
	for _, message := range globalChannelHistory {
		if len(message.PinnedTo) > 0 {
			ts := slackTSToHumanTime(message.Timestamp)
			status := getPinnedMessageStatus(message.Reactions)

			// Optionally send individual reminders.
			/*
			reminderText := fmt.Sprintf(
				"Hey, <@%s>, this message was posted on %s. It might be time to unpin it.",
				message.User,
				ts,
			)
			*/

			
			/*
			_, _, err := slackSocket.PostMessage(
				config.SlackStatusChannelID,
				slack.MsgOptionTS(message.Timestamp),
				slack.MsgOptionText(reminderText, false),
			)
			if err != nil {
				return err
			}
			*/

			// Grab permalink to send final reminder message.	
			permalink, err := slackSocket.GetPermalink(&slack.PermalinkParameters{
				Channel: config.SlackStatusChannelID,
				Ts:      message.Timestamp,
			})
			if err != nil {
				return err
			}
			pinnedMessageLinks = append(pinnedMessageLinks, ReminderInfo{permalink, ts, status})
		}
	}

	if len(pinnedMessageLinks) == 0 {
		fmt.Println("No messages pinned.")
		return nil
	}

	// Send summary message
	summaryMessage := fmt.Sprintln("<!here> Hello, Admins.\nThe following messages have been pinned for >1 day.")
	for _, m := range pinnedMessageLinks {
		var parsedStatus string
		if m.status == "" {
			parsedStatus = "•"
		} else {
			parsedStatus = fmt.Sprintf(":%s:", m.status)
		}

		summaryMessage += fmt.Sprintf("%s <%s|Since %s>\n\n", parsedStatus, m.link, m.ts)
	}

	summaryMessage += fmt.Sprintf("It might be time to unpin them if they are no longer relevant.")

	_, _, err := slackSocket.PostMessage(
		config.SlackStatusChannelID,
		slack.MsgOptionText(summaryMessage, false),
	)
	if err != nil {
		return err
	}
	return nil
}
