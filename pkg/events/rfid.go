package events

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
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
	addr     string
	listener net.Listener
	storage  *storage.BadgerStorage
}

func NewRfidReader(addr string, db *badger.DB) (*RfidReader, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to create new rfid listener: %w", err)
	}

	return &RfidReader{
		addr:     addr,
		listener: listener,
		storage:  storage.NewStorage(domain.EventEntity, db),
	}, nil
}

// Serve слушает события по tcp
func (rfid *RfidReader) Serve() {
	for {
		conn, err := rfid.listener.Accept()
		if err != nil {
			log.Err(err).Msg("tcp listener error")
			continue
		}

		log.Debug().Msg(":DEBUG new connection")
		go rfid.accept(conn)
	}
}

func (rfid *RfidReader) accept(conn net.Conn) {
	defer func() {
		if err := conn.Close(); err != nil {
			log.Err(err).Msg("failed to close rfid conn")
		}
	}()

	event := domain.Event{
		ID:        ksuid.New().String(),
		AntennaIP: conn.RemoteAddr().String(),
		RfidID:    "",
		Date:      time.Now(),
	}

	scanner := bufio.NewScanner(conn)
	scanner.Scan()
	line := strings.Split(scanner.Text(), ", ")
	if len(line) != 3 {
		log.Err(fmt.Errorf("invalid line size")).Msgf("line size: %v", len(line))
		return
	}

	event.TagID = strings.TrimSpace(line[0])
	antennaPosition, err := strconv.ParseInt(strings.TrimSpace(line[2]), 10, 64)
	if err != nil {
		log.Err(err).Msg("failed to parse antenna position")
		return
	}
	event.Antenna = antennaPosition

	discoveryTimePrepared, err := timeFromStringUnixMillis(strings.TrimSpace(line[1]))
	if err != nil {
		log.Err(err).Msg("failed to parse discovery time prepared")
		return
	}
	event.DiscoveryTimePrepared = discoveryTimePrepared

	if err = rfid.writeEvent(event); err != nil {
		log.Err(err).Msg("failed to write rfid event")
	}
}

func (rfid *RfidReader) writeEvent(event domain.Event) error {
	return rfid.storage.Create(event.ID, event)
}

func (rfid *RfidReader) Stop() {
	log.Printf("stop rfid reader, close db")
	if err := rfid.listener.Close(); err != nil {
		log.Err(err).Msg("failed to close rfid listener")
	}
}

func (rfid *RfidReader) ListEventsHttp(w http.ResponseWriter, r *http.Request) {
	events, err := rfid.storage.ListEvent(nil)
	if err != nil {
		return
	}

	rest.RenderJSON(w, events)
}

// string to time.Unix milli
func timeFromStringUnixMillis(ms string) (time.Time, error) {
	msInt, err := strconv.ParseInt(ms, 10, 64)
	if err != nil {
		return time.Time{}, err
	}

	return time.Unix(0, msInt*int64(time.Millisecond)), nil
}
