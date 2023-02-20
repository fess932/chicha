package storage

import (
	"fmt"

	"github.com/dgraph-io/badger/v3"
	"github.com/vmihailenco/msgpack/v5"

	"chicha/pkg/domain"
)

type BadgerStorage struct {
	entityPrefix []byte
	db           *badger.DB
}

func NewStorage(entityType string, db *badger.DB) *BadgerStorage {
	return &BadgerStorage{
		entityPrefix: []byte(entityType),
		db:           db,
	}
}

func (b *BadgerStorage) buildKey(key string) []byte {
	return []byte(fmt.Sprintf("%s/%s", string(b.entityPrefix), key))
}

func (b *BadgerStorage) buildValue(value interface{}) ([]byte, error) {
	buf, err := msgpack.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event:%w", err)
	}
	return buf, nil
}

func (b *BadgerStorage) Create(key string, value interface{}) error {
	buf, err := b.buildValue(value)
	if err != nil {
		return err
	}

	return b.db.Update(func(txn *badger.Txn) error {
		err = txn.Set(b.buildKey(key), buf)
		if err != nil {
			return err
		}
		return nil
	})
}

func (b *BadgerStorage) ListEvent(filterFunc func(event domain.Event) bool) ([]domain.Event, error) {
	var eventList []domain.Event
	if string(b.entityPrefix) != domain.EventEntity {
		return nil, fmt.Errorf("need entity: %s, has entity: %s", domain.EventEntity, b.entityPrefix)
	}

	err := b.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(b.entityPrefix); it.ValidForPrefix(b.entityPrefix); it.Next() {
			var e domain.Event
			if err := it.Item().Value(func(val []byte) error {
				return msgpack.Unmarshal(val, &e)
			}); err != nil {
				return err
			}
			if filterFunc != nil {
				if !filterFunc(e) {
					continue
				}
			}

			eventList = append(eventList, e)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get events list: %w", err)
	}

	return eventList, nil
}
