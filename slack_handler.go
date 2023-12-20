package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"github.com/tidwall/gjson"
)


type CSPSlackEvtHandler struct {
	*CSPSlack
	evt socketmode.Event
}

func (h *CSPSlackEvtHandler) handleEventAPIEvent() {
	eventsAPIEvent, ok := h.evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		fmt.Printf("Ignored %+v\n", h.evt)
		return
	}
	fmt.Printf("Event received: %+v\n", eventsAPIEvent)

	h.slackSocket.Ack(*h.evt.Request)

	switch eventsAPIEvent.Type {
	case slackevents.CallbackEvent:
		innerEvent := eventsAPIEvent.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.PinAddedEvent:
			h.shouldUpdate = true
		case *slackevents.PinRemovedEvent:
			h.shouldUpdate = true
		case *slackevents.ReactionRemovedEvent:
			if ev.User == config.SlackBotID {
				return
			}
			reaction := ev.Reaction
			h.slackSocket.RemoveReaction(reaction, slack.ItemRef{
				Channel:   config.SlackStatusChannelID,
				Timestamp: ev.Item.Timestamp,
			})
			h.shouldUpdate = true
		case *slackevents.ReactionAddedEvent:
			h.handleReactionAddedEvent(ev)
		case *slackevents.MessageEvent:
			h.handleMessageEvent(ev)
		default:
			log.Println("no handler for event of given type")
		}
	default:
	}
}

func (h *CSPSlackEvtHandler) handleReactionAddedEvent(ev *slackevents.ReactionAddedEvent) {
	reaction := ev.Reaction
	botMentioned, err := h.isBotMentioned(ev.Item.Timestamp)
	if err != nil {
		log.Println(err)
		return
	}
	if ev.User == config.SlackBotID || !isRelevantReaction(reaction) || (!botMentioned) {
		return
	}
	// If necessary, remove a conflicting reaction
	if isRelevantReaction(reaction) {
		h.clearReactions(
			ev.Item.Timestamp,
			[]string{
				config.StatusOKEmoji,
				config.StatusWarnEmoji,
				config.StatusErrorEmoji,
			},
		)
	}
	// Mirror the reaction on the message
	h.slackSocket.AddReaction(reaction, slack.NewRefToMessage(
		config.SlackStatusChannelID,
		ev.Item.Timestamp,
	))
	h.shouldUpdate = true
}

func (h *CSPSlackEvtHandler) handleMessageEvent(ev *slackevents.MessageEvent) {
	// If a message mentioning us gets added or deleted, then
	// do something
	log.Printf("Message type: %s\n", ev.SubType)

	// If the message was deleted, then update the page.
	// If LITERALLY ANYTHING ELSE happened, bail
	switch ev.SubType {
	case "": // continue
	case "message_deleted":
		h.shouldUpdate = true
		fallthrough
	default:
		return
	}

	// If the bot was mentioned in this message, then we should probably
	// re-build the page, and if not, we should bail.
	botID := fmt.Sprintf("<@%s>", config.SlackBotID)
	if strings.Contains(ev.Text, botID) {
		h.shouldUpdate = true
	} else {
		return
	}

	// HACK: If we're still here, it means we got mentioned, and should
	// do something about it. We do this instead of an AppMention because
	// there does not seem to be any way to not fire an AppMentionEvent
	// if a message is edited
	log.Printf("Got mentioned. Timestamp is: %s. ThreadTimestamp is: %s\n", ev.TimeStamp, ev.ThreadTimeStamp)

	channelName, err := h.resolveChannelName(config.SlackForwardChannelID)
	if err != nil {
		log.Printf("Could not resolve channel name: %s\n", err)
		return
	}
	blocks := CreateUpdateResponseMsg(channelName, ev.User)
	_, _, err = h.slackSocket.PostMessage(config.SlackStatusChannelID, slack.MsgOptionTS(ev.TimeStamp), slack.MsgOptionBlocks(blocks...))
	if err != nil {
		log.Printf("Error posting ephemeral message: %s", err)
	}
}

func (h *CSPSlackEvtHandler) handleInteractiveEvent() {
	callback, ok := h.evt.Data.(slack.InteractionCallback)
	if !ok {
		fmt.Printf("Ignored %+v\n", h.evt)
		return
	}

	var payload interface{}

	switch callback.Type {
	case slack.InteractionTypeBlockActions:
		// See https://api.slack.com/apis/connections/socket-implement#button
		// Check which button was pressed
		for _, action := range callback.ActionCallback.BlockActions {
			switch action.ActionID {
			case CSPSetOK, CSPSetWarn, CSPSetError, CSPCancel:
				h.handlePromptInteraction(callback, action)
			}
		}
	case slack.InteractionTypeShortcut:
		log.Printf("Got shortcut: %s", callback.CallbackID)
		if callback.CallbackID == CSPUpdateStatusPage {
			h.shouldUpdate = true
		}
	default:
		log.Println("no handler for event of given type")
	}

	h.slackSocket.Ack(*h.evt.Request, payload)
}

func (h *CSPSlackEvtHandler) handlePromptInteraction(callback slack.InteractionCallback, action *slack.BlockAction) {
	log.Printf("Block Action Detected: %s\n", action.ActionID)
	itemRef := slack.ItemRef{
		Channel:   callback.Channel.ID,
		Timestamp: callback.Container.ThreadTs,
	}

	selected_options := gjson.Get(string(callback.RawState), "values.options.options.selected_options").Array()
	for i, opt := range selected_options {
		fmt.Println(i, opt)

		option := gjson.Get(opt.String(), "value").String()
		switch option {
		case CSPPin:
			log.Println("Will pin message")

			// Get the conversation history
			err := h.slackSocket.AddPin(callback.Channel.ID, itemRef)
			if err != nil {
				log.Println(err)
			}
		case CSPForward:
			log.Println("Will forward message")

			messageText, err := h.getSingleMessage(callback.Channel.ID, callback.Container.ThreadTs)
			if err != nil {
				log.Println(err)
				break
			}

			botID := fmt.Sprintf("<@%s>", config.SlackBotID)
			strippedStatusUpdate := strings.Replace(messageText.Text, botID, "", -1)

			_, _, err = h.slackSocket.PostMessage(config.SlackForwardChannelID, slack.MsgOptionText(strippedStatusUpdate, false))
		}
	}

	// Clear any old reactions
	switch action.ActionID {
	case CSPSetOK, CSPSetWarn, CSPSetError:
		h.clearReactions(
			callback.Container.ThreadTs,
			[]string{
				config.StatusOKEmoji,
				config.StatusWarnEmoji,
				config.StatusErrorEmoji,
			},
		)
	}

	// Add the reaction we want
	switch action.ActionID {
	case CSPSetOK:
		err := h.slackSocket.AddReaction(config.StatusOKEmoji, itemRef)
		if err != nil {
			// Handle the error
			h.slackSocket.Debugf("Error adding reaction: %v", err)
		}
	case CSPSetWarn:
		err := h.slackSocket.AddReaction(config.StatusWarnEmoji, itemRef)
		if err != nil {
			// Handle the error
			h.slackSocket.Debugf("Error adding reaction: %v", err)
		}
	case CSPSetError:
		err := h.slackSocket.AddReaction(config.StatusErrorEmoji, itemRef)
		if err != nil {
			// Handle the error
			h.slackSocket.Debugf("Error adding reaction: %v", err)
		}
	case CSPCancel:
	}
	_, _, err := h.slackSocket.DeleteMessage(config.SlackStatusChannelID, callback.Container.MessageTs)
	if err != nil {
		log.Println(err)
	}
}

