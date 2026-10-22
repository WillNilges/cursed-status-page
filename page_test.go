package main

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"golang.org/x/net/html"
)

func nodeText(node *html.Node) string {
	var text strings.Builder
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return text.String()
}

func TestHelpMessagePreservesHistory(t *testing.T) {
	const message = `Having problems? <a href="https://example.com/slack">Support channel</a> or <a href="https://example.com/support">Support page</a>.`
	cases := []struct {
		name         string
		envValue     string
		wantHelpText string
		wantLinks    []string
	}{
		{"malformed double quotes", `"` + message + `"`, "Having problems?", nil},
		{"unquoted", message, "Having problems? Support channel or Support page.", []string{"https://example.com/slack", "https://example.com/support"}},
		{"single quotes", `'` + message + `'`, "Having problems? Support channel or Support page.", []string{"https://example.com/slack", "https://example.com/support"}},
		{"plain text", "Contact support.", "Contact support.", nil},
		{"empty", "", "", nil},
		{"unsafe HTML", `<script>alert(1)</script><a href="javascript:alert(1)">Support</a>`, "Support", nil},
	}
	originalConfig := config
	originalMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		config = originalConfig
		gin.SetMode(originalMode)
	})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			values, err := godotenv.Unmarshal("CSP_HELP_LINK=" + tc.envValue + "\n")
			if err != nil {
				t.Fatal(err)
			}
			config.HelpMessage = values["CSP_HELP_LINK"]
			if tc.name == "malformed double quotes" && !strings.HasSuffix(config.HelpMessage, "<a href=") {
				t.Fatalf("fixture did not reproduce the truncated help link: %q", config.HelpMessage)
			}
			page := CSPPage{updates: []StatusUpdate{{
				HTML: template.HTML(`<p><a href="/history-entry">History entry</a></p>`),
			}}}
			router := gin.New()
			router.LoadHTMLGlob("templates/*")
			router.GET("/", page.statusPage)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			doc, err := html.Parse(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			var help *html.Node
			historyCount, historyLinkCount := 0, 0
			var visit func(*html.Node)
			visit = func(n *html.Node) {
				if n.Type == html.ElementNode {
					if n.Data == "p" {
						for _, attr := range n.Attr {
							if attr.Key == "class" && attr.Val == "lead" {
								help = n
							}
						}
					}
					if n.Data == "em" && nodeText(n) == "History" {
						historyCount++
						for parent := n.Parent; parent != nil; parent = parent.Parent {
							if parent.Type == html.ElementNode && parent.Data == "a" {
								t.Error("history is inside an anchor from the help message")
							}
						}
					}
					for _, attr := range n.Attr {
						if n.Data == "a" && attr.Key == "href" && attr.Val == "/history-entry" {
							historyLinkCount++
						}
					}
				}
				for child := n.FirstChild; child != nil; child = child.NextSibling {
					visit(child)
				}
			}
			visit(doc)
			if historyCount != 1 || historyLinkCount != 1 {
				t.Errorf("history heading/links = %d/%d, want 1/1", historyCount, historyLinkCount)
			}
			if help == nil {
				t.Fatal("help paragraph is missing")
			}
			if got := strings.TrimSpace(nodeText(help)); got != tc.wantHelpText {
				t.Errorf("help text = %q, want %q", got, tc.wantHelpText)
			}
			links := map[string]int{}
			var inspectHelp func(*html.Node)
			inspectHelp = func(n *html.Node) {
				if n.Type == html.ElementNode {
					if n.Data == "script" {
						t.Error("script remained in the help message")
					}
					for _, attr := range n.Attr {
						if n.Data == "a" && attr.Key == "href" {
							links[attr.Val]++
						}
					}
				}
				for child := n.FirstChild; child != nil; child = child.NextSibling {
					inspectHelp(child)
				}
			}
			inspectHelp(help)
			if len(links) != len(tc.wantLinks) {
				t.Errorf("help links = %v, want %v", links, tc.wantLinks)
			}
			for _, link := range tc.wantLinks {
				if links[link] != 1 {
					t.Errorf("support link %q count = %d, want 1", link, links[link])
				}
			}
		})
	}
}
