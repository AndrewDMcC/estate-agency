package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/AndrewDMcC/estate-agency/internal/models"
)

var (
	bucketProperties = []byte("properties")
	bucketEnquiries  = []byte("enquiries")
	bucketSettings   = []byte("settings")
	bucketSecrets    = []byte("secrets")
	settingsKey      = []byte("site")
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db *bolt.DB
}

func Open(path string) (*Store, error) {
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open bolt db: %w", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, name := range [][]byte{bucketProperties, bucketEnquiries, bucketSettings, bucketSecrets, bucketRegistrations, bucketRightmoveQueue} {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create buckets: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func put(b *bolt.Bucket, key string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return b.Put([]byte(key), data)
}

func nextID(b *bolt.Bucket) (string, error) {
	n, err := b.NextSequence()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n), nil
}

func (s *Store) SaveProperty(p *models.Property) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketProperties)
		now := time.Now().UTC()
		if p.ID == "" {
			id, err := nextID(b)
			if err != nil {
				return err
			}
			p.ID = id
			p.CreatedAt = now
		}
		p.UpdatedAt = now
		return put(b, p.ID, p)
	})
}

func (s *Store) GetProperty(id string) (*models.Property, error) {
	var p models.Property
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketProperties).Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &p)
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) DeleteProperty(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketProperties).Delete([]byte(id))
	})
}

// ListProperties returns properties matching keep (all if nil), newest first.
func (s *Store) ListProperties(keep func(*models.Property) bool) ([]*models.Property, error) {
	var out []*models.Property
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketProperties).ForEach(func(_, v []byte) error {
			var p models.Property
			if err := json.Unmarshal(v, &p); err != nil {
				return err
			}
			if keep == nil || keep(&p) {
				out = append(out, &p)
			}
			return nil
		})
	})
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	return out, err
}

func (s *Store) SaveEnquiry(e *models.Enquiry) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketEnquiries)
		if e.ID == "" {
			id, err := nextID(b)
			if err != nil {
				return err
			}
			e.ID = id
		}
		if e.CreatedAt.IsZero() {
			e.CreatedAt = time.Now().UTC()
		}
		return put(b, e.ID, e)
	})
}

// ListEnquiries returns the newest enquiries first; limit <= 0 returns all.
func (s *Store) ListEnquiries(limit int) ([]*models.Enquiry, error) {
	var out []*models.Enquiry
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketEnquiries).Cursor()
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			var e models.Enquiry
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			out = append(out, &e)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
		return nil
	})
	return out, err
}

func (s *Store) CountEnquiries() (int, error) {
	var n int
	err := s.db.View(func(tx *bolt.Tx) error {
		n = tx.Bucket(bucketEnquiries).Stats().KeyN
		return nil
	})
	return n, err
}

func (s *Store) GetSettings() (models.Settings, error) {
	settings := models.DefaultSettings()
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketSettings).Get(settingsKey)
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &settings)
	})
	return settings, err
}

func (s *Store) SaveSettings(settings models.Settings) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return put(tx.Bucket(bucketSettings), string(settingsKey), settings)
	})
}

func (s *Store) hasSettings() (bool, error) {
	var ok bool
	err := s.db.View(func(tx *bolt.Tx) error {
		ok = tx.Bucket(bucketSettings).Get(settingsKey) != nil
		return nil
	})
	return ok, err
}

// Secret returns a persistent random hex secret, creating it on first use.
func (s *Store) Secret(name string) (string, error) {
	var secret string
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSecrets)
		if v := b.Get([]byte(name)); v != nil {
			secret = string(v)
			return nil
		}
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return err
		}
		secret = hex.EncodeToString(buf)
		return b.Put([]byte(name), []byte(secret))
	})
	return secret, err
}
