package main

import (
	"log"
	"testing"
)

func TestMrkdwnToHTMLAngleBrackets(t *testing.T) {
	// FIXME: Why is there a newline at the end???
	mrkdwnToHTMLStrings := map[string]string{
		"<>":                                  "<p>&lt;&gt;</p>\n",
		"Text after this &lt; less than sign": "<p>Text after this &lt; less than sign</p>\n",
		"Text after this&lt;less than sign with no spaces": "<p>Text after this&lt;less than sign with no spaces</p>\n",
		"Test String 04: Hello&lt;World&gt;":               "Test String 04: Hello<World>",
		"Test String 03: Hello &lt; World.":                "Test String 03: Hello < World.",
		"Test String 02: Hello&gt;World":                   "Test String 02: Hello<World",
		"Test String 01: Hello&lt;World":                   "Test String 01: Hello<World",
	}
	for m, h := range mrkdwnToHTMLStrings {
		htmlString := MrkdwnToHTML(m)
		log.Printf("What the fuck!?: %s\n", htmlString)
		if string(h) != string(htmlString) {
			t.Errorf("HTML String did not match.\nExpected: '%s'\nReceived: '%s'", h, htmlString)
			t.Fail()
		}
	}
}
