package main

import (
	"log"
	"testing"
)

func TestMrkdwnToHTMLAngleBrackets(t *testing.T) {
	// FIXME: Why is there a newline at the end???
	mrkdwnToHTMLStrings := map[string]string{
		"&lt;&gt;":                           "<p>&lt;&gt;</p>\n",
		"Test String 04: Hello&lt;World&gt;": "<p>Test String 04: Hello&lt;World&gt;</p>\n",
		"Test String 03: Hello &lt; World.":  "<p>Test String 03: Hello &lt; World.</p>\n",
		"Test String 02: Hello&gt;World":     "<p>Test String 02: Hello&gt;World</p>\n",
		"Test String 01: Hello&lt;World":     "<p>Test String 01: Hello&lt;World</p>\n",
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
