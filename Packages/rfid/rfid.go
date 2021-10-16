package rfid

import (
	"bytes"
	"chicha/Models"
	"encoding/csv"
	"encoding/xml"
	"io"
	"log"
	"net"
	"time"
)

type Listener struct {
	port string
	s    net.Listener

	ech chan Entry
}

func New(port string) *Listener {
	workers := 10
	return &Listener{port: port, ech: make(chan Entry, workers)}
}

type Entry struct { // todo lowercase
	Addr string
	Body []byte
}

func (l *Listener) Listen() {
	go l.server()

	s, err := net.Listen("tcp", l.port)
	if err != nil {
		log.Println(err)
		return
	}
	defer s.Close()
	l.s = s

	for {
		conn, err := s.Accept()
		if err != nil {
			log.Println("tcp accept error")
			continue
		}

		t := time.Now() // debug timer run

		addr, ok := conn.RemoteAddr().(*net.TCPAddr)
		if !ok {
			log.Println("wrong tcp addr:", addr)
		}

		b, err := io.ReadAll(conn)
		if err != nil {
			log.Println(err)
			conn.Close() // close conn if error
			continue
		}
		conn.Close() // close conn after read

		e := Entry{
			Addr: addr.IP.String(),
			Body: b,
		}

		l.ech <- e

		log.Println("time for one event:", time.Since(t)) // debug timer end
	}
}

func (l *Listener) server() {
	for {
		select { // graceful shutdown
		case e := <-l.ech:
			lap, err := decode(e.Body)
			if err != nil {
				log.Println("decode error:", err)
			}

			lap.AntennaIP = e.Addr
			// write to db, processing dublications, send to chan
			log.Println(lap)

		}
	}
}

func decode(data []byte) (Models.Lap, error) {
	if Models.IsValidXML(data) {
		return decodeXML(data)
	}

	return decodeCSV(data)
}

func decodeXML(data []byte) (lap Models.Lap, err error) {
	err = xml.Unmarshal(data, &lap)
	return
}

func decodeCSV(data []byte) (Models.Lap, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = ','
	r.FieldsPerRecord = 3
	CSV, err := r.Read()
	if err != nil {
		return Models.Lap{}, err
	}

	log.Println("csv::", CSV)

	return Models.Lap{}, nil
}

func decodeTime(str string) {

}
