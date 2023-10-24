# cursed-status-page
A simple status page webserver that you control using Slack

<p align="center">
  <img height="200px" src="https://github.com/WillNilges/cursed-status-page/assets/42927786/dae7f367-4351-4bb8-ad5a-d5008dc8ee0f" alt="Ugh">
</p>

![Static Badge](https://img.shields.io/badge/i_should_stop-writing_go-blue)
[![Docker](https://img.shields.io/badge/docker-hub-cyan)](https://hub.docker.com/repository/docker/willnilges/cursed-status-page/general)
[![Publish Docker Image](https://github.com/WillNilges/cursed-status-page/actions/workflows/to_docker_hub.yaml/badge.svg)](https://github.com/WillNilges/cursed-status-page/actions/workflows/to_docker_hub.yaml)
[![Deploy NYC Mesh Status Page](https://github.com/WillNilges/cursed-status-page/actions/workflows/deploy_to_nycmesh_status.yaml/badge.svg)](https://github.com/WillNilges/cursed-status-page/actions/workflows/deploy_to_nycmesh_status.yaml)


## Usage
Ping `@Cursed Status Page` in the `#status-page` channel with your status update, and then react to it with 🟢, 🟡, or 🔴 to turn it green, yellow, or red respectively. Pin the message in Slack to pin the message on the channel.

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

### Setup (Production)

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

