package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/slack-go/slack"
)

func generateSites(message slack.Message) (sites []Site, err error) {
	channelID := config.SlackStatusChannelID
	threadTs := message.Timestamp
	// Get the conversation history
	params := slack.GetConversationRepliesParameters{
		ChannelID: channelID,
		Timestamp: threadTs,
	}
	var siteMessages []slack.Message
	siteMessages, _, _, err = slackAPI.GetConversationReplies(&params)
	if err != nil {
		return sites, err
	}
	
	for _, message := range siteMessages {

		botID := fmt.Sprintf("<@%s>", config.SlackBotID)
		if strings.Contains(message.Text, botID) {
			continue
		}
		var site Site
		site.Name = message.Text
		site.Background = config.StatusNeutralColor
		for _, reaction := range message.Reactions {
			// Only take action on our reactions
			if botReaction := stringInSlice(reaction.Users, config.SlackBotID); !botReaction {
				continue
			}

			// Use the first reaction sent by the bot that we find
			switch reaction.Name {
			case config.StatusOKEmoji:
				site.Background = config.StatusOKColor
				break
			case config.StatusWarnEmoji:
				site.Background = config.StatusWarnColor
				break
			case config.StatusErrorEmoji:
				site.Background = config.StatusErrorColor
				break
			}
		}
		sites = append(sites, site)
	}

	return sites, nil
}

func buildStatusPage() (sites []Site, updates []StatusUpdate, pinnedUpdates []StatusUpdate, currentStatus StatusUpdate, err error) {
	log.Println("Building Status Page...")
	hasCurrentStatus := false
	for _, message := range globalChannelHistory {
		// FIXME (willnilges): Why is this teamID?
		teamID := fmt.Sprintf("<@%s>", config.SlackBotID)
		// Ignore messages that don't mention us. Also, ignore messages that
		// mention us but are empty!
		if !strings.Contains(message.Text, teamID) || message.Text == teamID {
			continue
		}
		// Check if there's a sites reaction
		for _, reaction := range message.Reactions {
			if reaction.Name == config.SiteEmoji {
				sites, err = generateSites(message)
				if err != nil {
					log.Println(err)
					return sites, updates, pinnedUpdates, currentStatus, err
				}
				continue
			}
		}
		msgUser, err := slackAPI.GetUserInfo(message.User)
		if err != nil {
			log.Println(err)
			return sites, updates, pinnedUpdates, currentStatus, err
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

	return sites, updates, pinnedUpdates, currentStatus, nil
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
			"Sites": globalSites, 
			"Org":            config.OrgName,
			"Logo":           config.LogoURL,
			"Favicon":        config.FaviconURL,
		},
	)
}

func health(c *gin.Context) {
	c.JSON(http.StatusOK, "cursed-status-page")
}
