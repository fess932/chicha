package main

import (
	"time"

	"github.com/rs/zerolog/log"

	"chicha/pkg/events"
)

func main() {
	evt, err := events.NewRfidReader("localhost:4000")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create rfid reader at localhost:4000")
	}

	evt.Serve()
	defer evt.Stop() // gracefull shutdown, wait until all data flushed to disk

	time.Sleep(time.Second * 10)
}
