package config

import (
	"io"
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// TracConfig represents a configured Trac server. This server will be queried
// for ticket information.
type TracConfig struct {
	// URL of the Trac instance, eg. http://mytrac.domain:8080
	URL string `yaml:"url"`

	// Username of the bot on that Trac instance
	Username string `yaml:"username"`

	// Password of the bot on that Trac instance
	Password string `yaml:"password"`

	// Whether to accept HTTPS from unknown authorities
	Insecure bool `yaml:"insecure"`

	// The authentication mechanism to use:
	// - http: HTTP Basic Auth
	// - form: Trac login form
	AuthType string `yaml:"auth_type"`
}

// ChannelConfig represents the configuration for a given channel. The
// Mattermost bot can join several channels, with a different configuration for
// each.
type ChannelConfig struct {
	// The list of Trac instances allowed from this channel
	TracInstances []string `yaml:"trac_instances"`

	// The default Trac instance to query if a bug ID is given without an
	// explicit Trac ID.
	DefaultTracInstance string `yaml:"default_trac_instance,omitempty"`
}

// Config is the main configuration of the Mattermost bot.
type Config struct {
	// URL of the Mattermost server, eg. http://server.domain:8080
	Server string `yaml:"server"`

	// Username of the bot on the Mattermost server
	Username string `yaml:"username"`

	// Password of the bot on the Mattermost server
	Password string `yaml:"password"`

	// Team of the bot on the Mattermost server
	Team string `yaml:"team"`

	// Go template (see the doc of template/text) for formatting ticket information
	TicketTemplate string `yaml:"ticket_template"`

	// List of configured Trac servers
	Tracs map[string]TracConfig `yaml:"tracs"`

	// Per-channel configuration
	Channels map[string]ChannelConfig `yaml:"channels"`
}

func LoadFromFile(filename string) (Config, error) {
	fd, err := os.Open(filename)

	if err != nil {
		return Config{}, errors.Wrapf(err, "Error while opening %s", filename)
	}

	defer fd.Close()

	c, err := Load(fd)

	if err != nil {
		return Config{}, errors.Wrapf(err, "Error while loading configuration from %s", filename)
	}

	return c, nil
}

func Load(r io.Reader) (Config, error) {
	configData, err := ioutil.ReadAll(r)

	if err != nil {
		return Config{}, errors.Wrap(err, "Error while reading configuration data")
	}

	c := Config{}

	if err := yaml.Unmarshal(configData, &c); err != nil {
		return Config{}, errors.Wrap(err, "YAML parse error")
	}

	if c.Tracs == nil {
		c.Tracs = map[string]TracConfig{}
	}

	if c.Channels == nil {
		c.Channels = map[string]ChannelConfig{}
	}

	if err := checkConfig(&c); err != nil {
		return Config{}, err
	}

	return c, nil
}

func checkConfig(c *Config) error {
	if len(c.Server) == 0 {
		return errors.New("Server field should not be empty")
	}

	if len(c.Username) == 0 {
		return errors.New("Username field should not be empty")
	}

	if len(c.Team) == 0 {
		return errors.New("Team field should not be empty")
	}

	for name, tracConfig := range c.Tracs {
		if len(tracConfig.URL) == 0 {
			return errors.Errorf("URL missing for Trac instance %s", name)
		}

		if len(tracConfig.Username) == 0 {
			return errors.Errorf("Username missing for Trac instance %s", name)
		}

		if len(tracConfig.Password) == 0 {
			return errors.Errorf("Password missing for Trac instance %s", name)
		}
	}

	for name, channelConfig := range c.Channels {
		if len(channelConfig.TracInstances) == 0 {
			return errors.Errorf("No Trac instances defined for channel %s", name)
		}

		for _, trac := range channelConfig.TracInstances {
			if _, ok := c.Tracs[trac]; !ok {
				return errors.Errorf("Trac instance %s referred from channel %s does not exist", trac, name)
			}
		}

		if len(channelConfig.DefaultTracInstance) > 0 {
			if _, ok := c.Tracs[channelConfig.DefaultTracInstance]; !ok {
				return errors.Errorf("Default Trac instance %s referred from channel %s does not exist", channelConfig.DefaultTracInstance, name)
			}
		}
	}

	return nil
}
