package main

import (
	"fmt"
	"time"

	"github.com/slack-go/slack"
)

type ReminderInfo struct {
	link   string
	ts     string
	status string
}

func (app *CSPSlack) SendReminders() error {
	fmt.Println("Sending unpin reminders...")
	var pinnedMessageLinks []ReminderInfo
	for _, message := range app.channelHistory {
		if len(message.PinnedTo) > 0 {
			ts := slackTSToHumanTime(message.Timestamp)
			status := GetPinnedMessageStatus(message.Reactions)

			// Don't bother if the message hasn't been up longer than a day
			t,  err := time.Parse("2006-01-02 15:04:05 MST", ts)
			if err == nil {
				if time.Since(t) < 24 * time.Hour {
					fmt.Println("Message not pinned for long enough. Ignoring.")
					continue
				}
			}

			// Optionally send individual reminders.
			// XXX: This seems like it would be too noisy.
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
			permalink, err := app.slackSocket.GetPermalink(&slack.PermalinkParameters{
				Channel: config.SlackStatusChannelID,
				Ts:      message.Timestamp,
			})
			if err != nil {
				return err
			}
			pinnedMessageLinks = append(pinnedMessageLinks, ReminderInfo{permalink, ts, status})
			fmt.Println("Found message.")
		}
	}

	if len(pinnedMessageLinks) == 0 {
		fmt.Println("No messages pinned.")
		return nil
	}

	// Send summary message
	summaryMessage := fmt.Sprintln("<!here> Hello, Admins.\nThe following messages are currently pinned.")
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

	_, _, err := app.slackSocket.PostMessage(
		config.SlackStatusChannelID,
		slack.MsgOptionText(summaryMessage, false),
	)
	if err != nil {
		return err
	}

	fmt.Println("success.")
	return nil
}
