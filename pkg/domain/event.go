package domain

import (
	"time"
)

const EventEntity = "EVENT"

// Event входящее событие из источника данных, rfid ридера например
type Event struct {
	ID         string
	RfidID     string
	Date       time.Time // время события из источника данных
	IncomeDate time.Time // время поступления события
}
