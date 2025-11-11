package main

import "testing"

func TestMrkdwnToHTMLAngleBrackets(t *testing.T) {
	sampleString := "<>"
	htmlString := MrkdwnToHTML(sampleString)
	// FIXME: Why is there a newline at the end???
	if string(htmlString) != "<p>&lt;&gt;</p>\n" {
		t.Errorf("HTML String did not match: '%s'", htmlString)
		t.Fail()
	}
}
