# cursed-status-page
A simple status page webserver that you control using Slack

<p align="center">
  <img height="200px" src="https://github.com/WillNilges/cursed-status-page/assets/42927786/dae7f367-4351-4bb8-ad5a-d5008dc8ee0f" alt="Ugh">
</p>

![Static Badge](https://img.shields.io/badge/i_should_stop-writing_go-blue)
[![Docker](https://img.shields.io/badge/docker-hub-cyan)](https://hub.docker.com/repository/docker/willnilges/cursed-status-page/general)


## Usage
Here's how it works:
- Post a message to status-page and ping `@Cursed Status Page`. The bot will reload its cache, pull the message, and display it on the "previous statuses" list.
- To pin it, react with 📌 . The bot will also react with that emoji, and the message will show up pinned. If there are more than 5 pinned, only the 5 most recent messages will show up pinned.
- To make it the current status, react with 🏆. The bot will react in kind. You can only have one current status, and the most recent 🏆'ed message will be the current one.
- To set a status color, react with ✅ , ⚠️ , or 🔥. The bot will react in kind.
- Basically, If a message already has a status set by someone else, you can simply react with a different status, and the bot will update accordingly.

## Setup

### Slack Bot

A manifest file is included with this repo.

Go to https://api.slack.com/apps and click, "Create New App", then upload
`slack-manifest.yaml` and update the URLs to point at wherever you host
your page.

### Setup (Development)

Clone this repo

```
git clone https://github.com/willnilges/cursed-status-page
``` 

Fill out the .env.sample

```
cp .env.sample .env
vim .env # Use your favorite editor
```

Build and Run

```
go run .
```

To serve this app, I use [ngrok](https://ngrok.com/)

```
ngrok http --domain <your-domain> --host-header=rewrite localhost:8080
```


### Setup (Dockerfile)

> [!WARNING]
> It is recommended to use the [Docker Image](https://hub.docker.com/repository/docker/willnilges/cursed-status-page/general)

This repo has a Dockerfile you can use

Clone this repo

```
git clone https://github.com/willnilges/cursed-status-page
``` 

Fill out the .env.sample

```
cp .env.sample .env
vim .env # Use your favorite editor
```

Build and Run

```
docker build . --tag cursed-status-page
docker run --rm --env-file .env -p 8080:8080 cursed-status-page
```
