package main

import (
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/abustany/mattermost-trac-bot/bot"
	"github.com/abustany/mattermost-trac-bot/config"
)

func main() {
	var configFile string
	var debug bool

	flag.StringVar(&configFile, "config", "", "Configuration file")
	flag.BoolVar(&debug, "debug", false, "Enable extra debugging logs")

	flag.Parse()

	if len(configFile) == 0 {
		log.Fatalf("Missing command line option: -config")
	}

	conf, err := config.LoadFromFile(configFile)

	if err != nil {
		log.Fatalf("Error while loading config file: %s", err)
	}

	sigCh := make(chan os.Signal, 1)
	errCh := make(chan error, 1)

	signal.Notify(sigCh, os.Interrupt)

	bot, err := bot.New(conf, debug)

	if err != nil {
		log.Fatalf("Error while starting client: %s", err)
	}

	go func() {
		errCh <- bot.Run()
	}()

	select {
	case <-sigCh:
		log.Printf("Received interrupt signal, doing a graceful shutdown")
	case err := <-errCh:
		if err != nil {
			log.Printf("Client error: %s", err)
		}
	}

	bot.Close()
}
