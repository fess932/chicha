package events

import (
	"net/http"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/go-pkgz/rest"
	"github.com/rs/zerolog/log"
	"github.com/segmentio/ksuid"

	"chicha/pkg/domain"
	"chicha/pkg/storage"
)

// низкоуровневый пакет для чтения всех входящих событий и хранения по ключам

// 1 читаем события из rfid по tcp
// 2 записываем в kv все события
// 3 снаружи должны уметь подписаться на изменения событий
// 4 по событиям нужно уметь построить состояние

type RfidReader struct {
	addr    string
	storage *storage.BadgerStorage
}

func NewRfidReader(addr string, db *badger.DB) (*RfidReader, error) {
	return &RfidReader{
		addr:    addr,
		storage: storage.NewStorage(domain.EventEntity, db),
	}, nil
}

// Serve слушает события по tcp
func (rfid *RfidReader) Serve() {
	for i := 0; i < 1000; i++ {
		if err := rfid.writeEvent(domain.Event{
			ID:         ksuid.New().String(),
			Date:       time.Now(),
			IncomeDate: time.Now().Add(time.Second),
		}); err != nil {
			log.Err(err).Msg("failed to serve")
		}
	}
}

func (rfid *RfidReader) writeEvent(event domain.Event) error {
	return rfid.storage.Create(event.ID, event)
}

func (rfid *RfidReader) Stop() {
	log.Printf("stop rfid reader, close db")
}

func (rfid *RfidReader) ListEventsHttp(w http.ResponseWriter, r *http.Request) {
	events, err := rfid.storage.ListEvent(nil)
	if err != nil {
		return
	}

	rest.RenderJSON(w, events)
}
