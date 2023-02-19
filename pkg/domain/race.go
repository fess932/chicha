package domain

import (
	"time"

	"github.com/segmentio/ksuid"
)

// todo: сделать разделение трасс по id rfid ридеров чтобы можно было
// запускать разные гонки в одно время

// новая гонка определяется так:
// - при старте программмы загружаются данные в кеш из бд
// - если в кеше нет гонок, создается новая гонка
// - если дата изменения последней гонки отстает от текущего на (120 сек), последняя гонка помечается как неактивная
// - при поступлении нового события создается новая гонка

type Race struct {
	ID        string
	CreatedAt time.Time
	UpdatedAt time.Time
	Active    bool
	Laps      []Lap // sorted lap list
}

func NewRace() *Race {
	return &Race{
		ID:        ksuid.New().String(), // todo: will panic in the zombie apocalypse
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Active:    false,
		Laps:      []Lap{}, // just for not nil
	}
}
