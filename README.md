# cursed-status-page
A simple status page webserver that you control using Slack

<p align="center">
  <img height="200px" src="https://github.com/WillNilges/cursed-status-page/assets/42927786/dae7f367-4351-4bb8-ad5a-d5008dc8ee0f" alt="Ugh">
</p>

[![GitHub go.mod Go version of a Go module](https://img.shields.io/github/go-mod/go-version/willnilges/grab.svg)](https://github.com/willnilges/grab)
[![Run Tests](https://github.com/WillNilges/cursed-status-page/actions/workflows/run_tests.yaml/badge.svg)](https://github.com/WillNilges/cursed-status-page/actions/workflows/run_tests.yaml)
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

### Help message configuration

Keep `CSP_HELP_LINK` unquoted as shown in `.env.sample`. This works with both
the application's godotenv loader and Docker's `--env-file` option:

```dotenv
CSP_HELP_LINK=Need help? <a href="https://nycmesh.net/support">Contact support</a>.
```

For a `.env` file read directly by godotenv, single outer quotes are also valid:

```dotenv
CSP_HELP_LINK='Need help? <a href="https://nycmesh.net/support">Contact support</a>.'
```

Use the unquoted form with Docker `--env-file`, which can preserve outer quotes
as part of the value. Do not wrap HTML containing double-quoted attributes in
unescaped double outer quotes: godotenv can truncate the value at the first
attribute quote without reporting an error.

The help message uses the same bluemonday safe-HTML policy as status messages.
Normal support links and supported HTML are preserved; unsupported markup and
unsafe attributes are filtered. This prevents an incomplete help link from
affecting the history below it, but cannot recover text already lost while
loading the configuration.

