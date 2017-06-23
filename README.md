# Mattermost Trac Bot

The Mattermost Trac Bot connects to a Mattermost server, listens on some
channels for ticket IDs, and replies to the messages with information about the
tickets.

A typical conversation could look like:

```
<user1>     @user2 have you fixed ticket #35 yet ?
<trac_bot>  Ticket 35 (defect, new) — Panic in backend code
<user2>     Not yet, going to take a look right now!
```

## Features

- Can connect to one or several Trac instances, using either HTTP or form based
  authentication
- Can listen to an arbitrary number of channels, and be configured to allow only
  certain channels to query certain Trac instances
- Easy to install, well documented: compiles to a single, static binary, and
  shipped with a comprehensively documented configuration file.

## Installation

Until there are binary releases, you need the Go toolchain to get and compile
the binary:

```
GOPATH=$(pwd) go get github.com/abustany/mattermost-trac-bot
```

If everything went well, you should have a `mattermost-trac-bot` binary in the
`bin` directory. Download the sample `config.yaml` file from this repository,
adjust it to your needs, and start the bot with

```
./bin/mattermost-trac-bot -config config.yaml
```
