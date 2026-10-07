package store

import (
	"encoding/json"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/AndrewDMcC/estate-agency/internal/models"
)

var (
	bucketRegistrations = []byte("registrations")
	registerOptionsKey  = []byte("register_options")
)

func (s *Store) SaveRegistration(reg *models.Registration) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketRegistrations)
		if reg.ID == "" {
			id, err := nextID(b)
			if err != nil {
				return err
			}
			reg.ID = id
		}
		if reg.CreatedAt.IsZero() {
			reg.CreatedAt = time.Now().UTC()
		}
		return put(b, reg.ID, reg)
	})
}

// ListRegistrations returns the newest registrations first; limit <= 0 returns all.
func (s *Store) ListRegistrations(limit int) ([]*models.Registration, error) {
	var out []*models.Registration
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketRegistrations).Cursor()
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			var reg models.Registration
			if err := json.Unmarshal(v, &reg); err != nil {
				return err
			}
			out = append(out, &reg)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
		return nil
	})
	return out, err
}

func (s *Store) CountRegistrations() (int, error) {
	var n int
	err := s.db.View(func(tx *bolt.Tx) error {
		n = tx.Bucket(bucketRegistrations).Stats().KeyN
		return nil
	})
	return n, err
}

func (s *Store) GetRegisterOptions() (models.RegisterOptions, error) {
	opts := models.DefaultRegisterOptions()
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketSettings).Get(registerOptionsKey)
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &opts)
	})
	return opts, err
}

func (s *Store) SaveRegisterOptions(opts models.RegisterOptions) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return put(tx.Bucket(bucketSettings), string(registerOptionsKey), opts)
	})
}
