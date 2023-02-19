package main

import (
	"time"

	"github.com/rs/zerolog/log"

	"chicha/pkg/events"
)

func main() {
	evt, err := events.NewRfidReader("localhost:4000")
	if err != nil {
		log.Err(err).Msg("failed to create rfid reader at localhost:4000")
		return
	}

	evt.Serve()

	time.Sleep(time.Second * 2000)

	// gracefull shutdown
	defer func() {
		evt.Stop() // gracefull shutdown, wait until all data flushed to disk
		log.Info().Msg("chicha stopped")
	}()
}
