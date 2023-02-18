package domain

import "time"

// Event входящее событие из источника данных, rfid ридера например
type Event struct {
	Date       time.Time // время события из источника данных
	IncomeDate time.Time // время поступления события
}
