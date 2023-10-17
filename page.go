package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func buildStatusPage() (updates []StatusUpdate, pinnedUpdates []StatusUpdate, currentStatus StatusUpdate, err error) {
	log.Println("Building Status Page...")
	hasCurrentStatus := false
	for _, message := range globalChannelHistory {
		teamID := fmt.Sprintf("<@%s>", config.SlackBotID)
		// Ignore messages that don't mention us. Also, ignore messages that
		// mention us but are empty!
		if !strings.Contains(message.Text, teamID) || message.Text == teamID {
			continue
		}
		msgUser, err := slackAPI.GetUserInfo(message.User)
		if err != nil {
			log.Println(err)
			return updates, pinnedUpdates, currentStatus, err
		}
		realName := msgUser.RealName
		var update StatusUpdate
		update.Text = strings.Replace(message.Text, teamID, "", -1)
		update.SentBy = realName
		update.TimeStamp = slackTSToHumanTime(message.Timestamp)
		update.Background = config.StatusNeutralColor

		willBeCurrentStatus := false
		shouldPin := false
		for _, reaction := range message.Reactions {
			// Only take action on our reactions
			if botReaction := stringInSlice(reaction.Users, config.SlackBotID); !botReaction {
				continue
			}
			// If we find a pin at all, then use it
			if reaction.Name == config.CurrentEmoji && hasCurrentStatus == false {
				willBeCurrentStatus = true
			} else if reaction.Name == config.PinEmoji {
				shouldPin = true
			}

			// Use the first reaction sent by the bot that we find
			if update.Background == config.StatusNeutralColor {
				switch reaction.Name {
				case config.StatusOKEmoji:
					update.Background = config.StatusOKColor
				case config.StatusWarnEmoji:
					update.Background = config.StatusWarnColor
				case config.StatusErrorEmoji:
					update.Background = config.StatusErrorColor
				}
			}
		}
		if willBeCurrentStatus {
			currentStatus = update
			hasCurrentStatus = true
		} else if shouldPin && len(pinnedUpdates) < config.PinLimit {
			pinnedUpdates = append(pinnedUpdates, update)
		} else {
			updates = append(updates, update)
		}
	}

	if !hasCurrentStatus {
		currentStatus = StatusUpdate{
			Text:       config.NominalMessage,
			SentBy:     config.NominalSentBy,
			TimeStamp:  "Now",
			Background: config.StatusOKColor,
		}
	}

	return updates, pinnedUpdates, currentStatus, nil
}

func statusPage(c *gin.Context) {
	c.HTML(
		http.StatusOK,
		"index.html",
		gin.H{
			"HelpMessage":    template.HTML(config.HelpMessage),
			"PinnedStatuses": globalPinnedUpdates,
			"CurrentStatus":  globalCurrentStatus,
			"StatusUpdates":  globalUpdates,
			"Org":            config.OrgName,
			"Logo":           config.LogoURL,
			"Favicon":        config.FaviconURL,
		},
	)
}

func health(c *gin.Context) {
	c.JSON(http.StatusOK, "cursed-status-page")
}
