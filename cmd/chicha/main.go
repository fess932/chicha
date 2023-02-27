package main

import (
	"net/http"

	"github.com/dgraph-io/badger/v3"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog/log"

	"chicha/Packages/Config"
	"chicha/pkg/events"
)

func main() {
	// open database
	opts := badger.DefaultOptions("./binaries/badgerdb").WithLoggingLevel(badger.ERROR)
	db, err := badger.Open(opts)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open db")
	}

	rfidReader, err := events.NewRfidReader(Config.APP_ANTENNA_LISTENER_IP, db)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create rfid reader at localhost:4000")
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/", rfidReader.ListEventsHttp)

	// gracefull shutdown
	defer func() {
		rfidReader.Stop() // gracefull shutdown, event listener

		// database compact and stop
		log.Err(db.Flatten(4)).Msg("flatten on stop")
		log.Err(db.RunValueLogGC(0.5)).Msg("run value log gc")
		if err = db.Close(); err != nil {
			log.Err(err).Msg("failed to close badger db")
		}

		log.Info().Msg("chicha stopped")
	}()

	go rfidReader.Serve() // старт рфид ридера

	// старт http сервера для клиентов
	if err = http.ListenAndServe("localhost:8080", r); err != nil {
		log.Err(err).Msg("failed to close")
		return
	}
}
