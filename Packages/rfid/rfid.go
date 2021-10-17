package rfid

import (
	"chicha/Packages/Config"
	"chicha/Packages/rfid/models"
	"chicha/Packages/rfid/uniqer"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

type Listener struct {
	port string

	ech chan Entry

	Lap <-chan models.AverageLap
}

var once sync.Once

func New(port string) *Listener {
	workers := 10
	loc, err := time.LoadLocation(Config.TIME_ZONE)
	if err != nil {
		log.Fatal(err)
	}

	tz = loc
	in := make(chan models.Lap)
	l := &Listener{port: port, ech: make(chan Entry, workers), Lap: uniqer.Uniq(in)}
	go l.server(in)

	return l
}

type Entry struct { // todo lowercase
	Addr string
	Body []byte
}

func (l *Listener) Listen() {
	go log.Println("start listen at", l.port)
	s, err := net.Listen("tcp", l.port)
	if err != nil {
		log.Println(err)
		return
	}
	defer s.Close()

	for {
		conn, err := s.Accept()
		if err != nil {
			log.Println("tcp accept error")
			continue
		}

		//t := time.Now() // debug timer run

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

		// add to log
		// logger.write(e)

		// process data
		go l.serve(Entry{
			Addr: addr.IP.String(),
			Body: b,
		})

		//log.Println("time for one event:", time.Since(t)) // debug timer end
	}
}

func (l *Listener) serve(e Entry) {
	l.ech <- e
}

func (l *Listener) server(in chan models.Lap) {
	for {
		select { // graceful shutdown
		case e := <-l.ech:
			lap, err := decode(e.Body)
			if err != nil {
				log.Println("decode error:", err)
			}

			lap.AntennaIP = e.Addr

			// send to uniqer
			// write to db, processing dublications, send to chan
			in <- lap
		}
	}
}
