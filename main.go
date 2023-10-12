package main

import (
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"

	"github.com/gin-gonic/gin"
)


type Config struct {
	SlackTeamID string
	SlackAccessToken string
}

var config Config


func init() {
	// Load environment variables, one way or another
	err := godotenv.Load()
	if err != nil {
		log.Println("Couldn't load .env file")
	}

	config.SlackTeamID = os.Getenv("CSP_SLACK_TEAMID")
	config.SlackAccessToken = os.Getenv("CSP_SLACK_ACCESS_TOKEN")
}

func hello(c *gin.Context) {
	c.HTML(http.StatusOK, "index.html", gin.H{})
}

func main() {
	app := gin.Default()
	app.LoadHTMLGlob("templates/*")
	app.Static("/static", "./static")

	slackGroup := app.Group("/slack")

	//installGroup := slackGroup.Group("/install")
	//// First, the user goes to the form to submit mediawiki creds
	//installGroup.GET("/", func(c *gin.Context) {
	//	slackError := c.DefaultQuery("error", "")
	//	if slackError != "" {
	//		slackErrorDescription := c.Query("error_description")
	//		c.HTML(http.StatusOK, "error.html", gin.H{
	//			"SlackError": slackError,
	//			"ErrorDesc":  slackErrorDescription,
	//		})
	//		return
	//	}

	//	code := c.DefaultQuery("code", "") // Retrieve the code parameter from the query string
	//	c.HTML(http.StatusOK, "index.html", gin.H{
	//		"Code": code, // Pass the code parameter to the template
	//	})
	//})

	//// Then, the creds get submitted
	//installGroup.POST("/submit", func(c *gin.Context) {
	//	wikiUsername := c.PostForm("username")
	//	wikiPassword := c.PostForm("password")
	//	wikiUrl := c.PostForm("url")
	//	wikiDomain := c.PostForm("domain")
	//	code := c.PostForm("code")

	//	c.Redirect(
	//		http.StatusSeeOther,
	//		"/slack/install/authorize?code="+code+"&mediaWikiUname="+wikiUsername+"&mediaWikiPword="+wikiPassword+"&mediaWikiURL="+wikiUrl+"&mediaWikiDomain="+wikiDomain,
	//	)
	//})
	//// Then we use them while we set up the DB and do Slack things
	//installGroup.Any("/authorize", installResp())

	// Serve initial interactions with the bot
	eventGroup := slackGroup.Group("/event")
	eventGroup.Use(signatureVerification)
	eventGroup.POST("/handle", eventResp())

	interactionGroup := slackGroup.Group("/interaction")
	interactionGroup.Use(signatureVerification)
	interactionGroup.POST("/handle", interactionResp())

	app.GET("/", hello)

	_ = app.Run()
}
