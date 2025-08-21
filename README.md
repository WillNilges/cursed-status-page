# cursed-status-page
A simple status page webserver that you control using Slack

<p align="center">
  <img height="200px" src="https://github.com/WillNilges/cursed-status-page/assets/42927786/dae7f367-4351-4bb8-ad5a-d5008dc8ee0f" alt="Ugh">
</p>

[![GitHub go.mod Go version of a Go module](https://img.shields.io/github/go-mod/go-version/willnilges/grab.svg)](https://github.com/willnilges/grab)
[![Docker](https://img.shields.io/badge/docker-hub-cyan)](https://hub.docker.com/repository/docker/willnilges/cursed-status-page/general)
[![Publish Docker Image](https://github.com/WillNilges/cursed-status-page/actions/workflows/to_docker_hub.yaml/badge.svg)](https://github.com/WillNilges/cursed-status-page/actions/workflows/to_docker_hub.yaml)
[![Deploy NYC Mesh Status Page](https://github.com/WillNilges/cursed-status-page/actions/workflows/deploy_to_nycmesh_status.yaml/badge.svg)](https://github.com/WillNilges/cursed-status-page/actions/workflows/deploy_to_nycmesh_status.yaml)


## Usage
Ping `@Status` to update
https://status.nycmesh.net/

Use reactions to change colors.
✅ white_check_mark
⚠️ warning
🔥 fire

Pin a message to the channel to pin it to the page.

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

#### Certificates

Set up your .env file and move it into `deploy/`

Run the `init-letsencrypt.sh` script with the following arguments:

`./init-letsencrypt.sh <domain> <email_address> real`

For domain, the nginx config is set up for status.nycmesh.net.

*If you need to change the domain, update the nginx config too.*

Go to your webpage and assuming everything is setup correctly, you should see
the status page running.

*TODO: Document auto-updating when the app builds + pushes.*

#### With Dockerfile

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

