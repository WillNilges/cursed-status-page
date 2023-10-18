package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func buildStatusPage() (updates []StatusUpdate, pinnedUpdates []StatusUpdate, err error) {
	log.Println("Building Status Page...")
	for _, message := range globalChannelHistory {
		botID := fmt.Sprintf("<@%s>", config.SlackBotID)
		// Ignore messages that don't mention us. Also, ignore messages that
		// mention us but are empty!
		if !strings.Contains(message.Text, botID) || message.Text == botID {
			continue
		}

		msgUser, err := slackSocket.GetUserInfo(message.User)
		if err != nil {
			log.Println(err)
			return updates, pinnedUpdates, err
		}
		realName := msgUser.RealName
		var update StatusUpdate
		update.Text = strings.Replace(message.Text, botID, "", -1)
		update.SentBy = realName
		update.TimeStamp = slackTSToHumanTime(message.Timestamp)
		update.BackgroundClass = ""
		update.IconFilename = ""

		for _, reaction := range message.Reactions {
			// Only take action on our reactions
			if botReaction := stringInSlice(reaction.Users, config.SlackBotID); !botReaction {
				continue
			}

		// Use the first reaction sent by the bot that we find
			switch reaction.Name {
			case config.StatusOKEmoji:
				update.BackgroundClass = "list-group-item-success"
				update.IconFilename = "checkmark.svg"
			case config.StatusWarnEmoji:
				update.BackgroundClass = "list-group-item-warning"
				update.IconFilename = "warning.svg"
			case config.StatusErrorEmoji:
				update.BackgroundClass = "list-group-item-danger"
				update.IconFilename = "error.svg"
			}

		}
		if len(message.PinnedTo) > 0 {
			pinnedUpdates = append(pinnedUpdates, update)
		} else {
			updates = append(updates, update)
		}
	}

	return updates, pinnedUpdates, nil
}

func statusPage(c *gin.Context) {
	c.HTML(
		http.StatusOK,
		"index.html",
		gin.H{
			"HelpMessage":    template.HTML(config.HelpMessage),
			"PinnedStatuses": globalPinnedUpdates,
			"StatusUpdates":  globalUpdates,
			"Org":            config.OrgName,
			"Logo":           config.LogoURL,
			"Favicon":        config.FaviconURL,
			"NominalMessage": config.NominalMessage,
		},
	)
}

func health(c *gin.Context) {
	c.JSON(http.StatusOK, "cursed-status-page")
}
