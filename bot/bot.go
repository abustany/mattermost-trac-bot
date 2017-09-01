package bot

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"
	"sync"
	"text/template"

	"github.com/mattermost/platform/model"
	"github.com/pkg/errors"

	"github.com/abustany/mattermost-trac-bot/config"
	"github.com/abustany/mattermost-trac-bot/trac"
)

type Bot struct {
	sync.Mutex

	conf           config.Config
	ticketTemplate *template.Template
	client         *model.Client
	wsClient       *model.WebSocketClient
	user           *model.User

	// Base data loaded just after connecting
	globalInfo *model.InitialLoad

	// Maps channel names to Channel objects
	channels map[string]*model.Channel

	// Maps a channel ID to its name
	channelNames map[string]string

	// Maps normalized trac IDs to original ones
	tracs map[string]*trac.Client
}

var TICKET_RE = regexp.MustCompile(`([a-zA-Z0-9]+)?#(\d+)`)

func New(conf config.Config, debug bool) (*Bot, error) {
	tracs := map[string]*trac.Client{}

	ticketTemplate, err := template.New("ticket").Parse(conf.TicketTemplate)

	if err != nil {
		return nil, errors.Wrap(err, "Error while compiling ticket formatting template")
	}

	for name, config := range conf.Tracs {
		id := strings.ToLower(name)

		if _, ok := tracs[id]; ok {
			return nil, errors.Errorf("Conflicting Trac name for %s", name)
		}

		authType, err := trac.ParseAuthType(config.AuthType)

		if err != nil {
			return nil, errors.Wrapf(err, "Invalid authentication type for Trac %s", name)
		}

		log.Printf("Setting up Trac client %s with auth %s", name, config.AuthType)

		client, err := trac.New(config.URL, authType, debug)

		if err != nil {
			return nil, errors.Wrap(err, "Error while initializing Trac client")
		}

		client.SetInsecure(config.Insecure)

		if err := client.Authenticate(config.Username, config.Password); err != nil {
			return nil, errors.Wrapf(err, "Authentication error for Trac %s", name)
		}

		tracs[id] = client
	}

	return &Bot{
		conf:           conf,
		ticketTemplate: ticketTemplate,
		client:         model.NewClient(conf.Server),
		channels:       map[string]*model.Channel{},
		channelNames:   map[string]string{},
		tracs:          tracs,
	}, nil
}

func makeTracIds(ids map[string]config.TracConfig) map[string]string {
	normalizedIds := make(map[string]string, len(ids))

	for id, _ := range ids {
		normalizedIds[strings.ToLower(id)] = id
	}

	return normalizedIds
}

func (b *Bot) Run() error {
	if props, err := b.client.GetPing(); err != nil {
		return errors.Wrap(err, "Error while pinging the server")
	} else {
		log.Printf("Mattermost server version %s", props["version"])
	}

	if res, err := b.client.Login(b.conf.Username, b.conf.Password); err != nil {
		return errors.Wrapf(err, "Error while logging in as %s", b.conf.Username)
	} else {
		log.Printf("Logged in as %s", b.conf.Username)
		b.user = res.Data.(*model.User)
	}

	if res, err := b.client.GetInitialLoad(); err != nil {
		return errors.Wrap(err, "Error while loading initial data")
	} else {
		log.Printf("Loaded initial data")
		b.globalInfo = res.Data.(*model.InitialLoad)
	}

	var botTeam *model.Team

	for _, team := range b.globalInfo.Teams {
		if team.Name == b.conf.Team {
			botTeam = team
			break
		}
	}

	if botTeam == nil {
		return errors.Errorf("Found no team named %s", b.conf.Team)
	}

	b.client.SetTeamId(botTeam.Id)

	if err := b.loadChannels(); err != nil {
		return errors.Wrap(err, "Error while setting up channels")
	}

	if err := b.handleWebSocket(); err != nil {
		return errors.Wrap(err, "Error while starting WebSockets client")
	}

	return nil
}

func (b *Bot) loadChannels() error {
	res, err := b.client.GetChannels("")

	if err != nil {
		return errors.Wrap(err, "Error while listing channels")
	}

	for _, serverChan := range *res.Data.(*model.ChannelList) {
		if _, ok := b.conf.Channels[serverChan.Name]; ok {
			b.channels[serverChan.Name] = serverChan
			b.channelNames[serverChan.Id] = serverChan.Name
		}
	}

	for c, _ := range b.conf.Channels {
		if _, ok := b.channels[c]; !ok {
			return errors.Errorf("No channel %s on server", c)
		}
	}

	return nil
}

func (b *Bot) handleWebSocket() error {
	if !strings.HasPrefix(b.conf.Server, "http") || len(b.conf.Server) < 5 {
		return errors.Errorf("Server URL is not HTTP?!")
	}

	wsUrl := "ws" + b.conf.Server[4:]

	log.Printf("Connecting to %s", wsUrl)

	if wsClient, err := model.NewWebSocketClient(wsUrl, b.client.AuthToken); err != nil {
		return errors.Wrapf(err, "Error while establishing connection to %s", wsUrl)
	} else {
		b.Lock()
		b.wsClient = wsClient
		b.Unlock()
	}

	b.wsClient.Listen()

	for ev := range b.wsClient.EventChannel {
		if _, ok := b.channelNames[ev.Broadcast.ChannelId]; !ok {
			continue
		}

		if ev.Event != model.WEBSOCKET_EVENT_POSTED {
			continue
		}

		post := model.PostFromJson(strings.NewReader(ev.Data["post"].(string)))

		if post == nil || post.UserId == b.user.Id {
			continue
		}

		if err := b.handleMessage(post); err != nil {
			log.Printf("Error while handling post %s: %s", post.Id, err)
		}
	}

	return nil
}

func stringSliceContainsNC(slice []string, needle string) bool {
	needle = strings.ToLower(needle)

	for _, s := range slice {
		if strings.ToLower(s) == needle {
			return true
		}
	}

	return false
}

func (b *Bot) handleMessage(post *model.Post) error {
	matches := TICKET_RE.FindAllStringSubmatch(post.Message, -1)

	if matches == nil {
		return nil
	}

	channelName := b.channelNames[post.ChannelId]
	channelConfig := b.conf.Channels[channelName]

	message := bytes.NewBuffer(nil)

	for _, match := range matches {
		tracId := match[1]
		ticketNumber := match[2]

		ticket, err := b.handleTicketRequest(channelConfig, tracId, ticketNumber)

		if err != nil {
			err = formatErrorMessage(message, err)
		} else {
			err = formatTicketMessage(message, b.ticketTemplate, ticket)
		}

		if err != nil {
			return errors.Wrap(err, "Error while formatting ticket data")
		}

		message.WriteString("\n")
	}

	reply := model.Post{}
	reply.ChannelId = post.ChannelId
	reply.Message = message.String()

	if _, err := b.client.CreatePost(&reply); err != nil {
		return errors.Wrapf(err, "Error while sending message on channel %s", channelName)
	}

	return nil
}

func formatErrorMessage(w io.Writer, err error) error {
	fmt.Fprintf(w, ":x: %s", err.Error())
	return nil
}

func formatTicketMessage(w io.Writer, tmpl *template.Template, t trac.Ticket) error {
	return errors.Wrap(tmpl.Execute(w, t), "Error while rendering ticket template")
}

func (b *Bot) handleTicketRequest(channelConfig config.ChannelConfig, tracId string, ticketNumber string) (trac.Ticket, error) {
	if len(tracId) == 0 {
		if len(channelConfig.DefaultTracInstance) > 0 {
			tracId = channelConfig.DefaultTracInstance
		} else {
			return trac.Ticket{}, errors.Errorf("Missing Trac ID for ticket #%s", ticketNumber)
		}
	}

	if !stringSliceContainsNC(channelConfig.TracInstances, tracId) {
		return trac.Ticket{}, errors.Errorf("Trac ID %s not configured for this channel", tracId)
	}

	client := b.tracs[strings.ToLower(tracId)]

	if client == nil {
		return trac.Ticket{}, errors.Errorf("Unknown Trac ID: %s", tracId)
	}

	ticket, err := client.GetTicket(ticketNumber)

	if err != nil {
		return trac.Ticket{}, errors.Wrapf(err, "Error while retrieving ticket %s#%s", tracId, ticketNumber)
	}

	return ticket, nil
}

func (b *Bot) Close() {
	b.Lock()
	if b.wsClient != nil {
		b.wsClient.Close()
	}
	b.Unlock()
}
