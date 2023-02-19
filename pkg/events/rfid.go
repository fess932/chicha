package events

import (
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/rs/zerolog/log"
	"github.com/segmentio/ksuid"
	"github.com/vmihailenco/msgpack/v5"

	"chicha/pkg/domain"
)

// низкоуровневый пакет для чтения всех входящих событий и хранения по ключам

// 1 читаем события из rfid по tcp
// 2 записываем в kv все события
// 3 снаружи должны уметь подписаться на изменения событий
// 4 по событиям нужно уметь построить состояние

type RfidReader struct {
	addr string
	db   *badger.DB
}

func NewRfidReader(addr string) (*RfidReader, error) {
	opts := badger.DefaultOptions("./binaries/badgerdb").WithLoggingLevel(badger.ERROR)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open db: %w", err)
	}

	return &RfidReader{
		addr: addr,
		db:   db,
	}, nil
}

// Serve слушает события по tcp
func (r *RfidReader) Serve() {
	for i := 0; i < 1000; i++ {
		if err := r.writeEvent(domain.Event{
			ID:         ksuid.New().String(),
			Date:       time.Now(),
			IncomeDate: time.Now().Add(time.Second),
		}); err != nil {
			log.Err(err).Msg("failed to serve")
		}
	}
}

func (r *RfidReader) Stop() {
	log.Printf("stop rfid reader, close db")
	log.Err(r.db.Flatten(4)).Msg("flatten on stop")
	log.Err(r.db.RunValueLogGC(0.5)).Msg("run value log gc")

	if err := r.db.Close(); err != nil {
		log.Err(err).Msg("failed to stop badger db")
	}
}

func (r *RfidReader) writeEvent(event domain.Event) error {
	buf, err := msgpack.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event:%w", err)
	}

	return r.db.Update(func(txn *badger.Txn) error {
		err = txn.Set([]byte(event.ID), buf)
		if err != nil {
			return err
		}
		return nil
	})
}
