package store

import (
	"encoding/json"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/AndrewDMcC/estate-agency/internal/models"
)

var bucketRightmoveQueue = []byte("rightmove_queue")

// RightmoveJob asks the worker to bring one property's Rightmove listing up to date.
// There is at most one job per property; re-queuing replaces it.
type RightmoveJob struct {
	PropertyID  string    `json:"property_id"`
	AgentRef    string    `json:"agent_ref"`
	Version     uint64    `json:"version"`
	Attempts    int       `json:"attempts"`
	NextAttempt time.Time `json:"next_attempt"`
}

// EnqueueRightmove queues a property for sending now. agentRef is used to remove the
// listing if the property has been deleted by the time the job runs.
func (s *Store) EnqueueRightmove(propertyID, agentRef string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		q := tx.Bucket(bucketRightmoveQueue)
		v, err := q.NextSequence()
		if err != nil {
			return err
		}
		job := RightmoveJob{PropertyID: propertyID, AgentRef: agentRef, Version: v, NextAttempt: time.Now().UTC()}
		if err := put(q, propertyID, job); err != nil {
			return err
		}
		return updateRightmoveSync(tx, propertyID, func(rs *models.RightmoveSync) { rs.Pending = true })
	})
}

// DueRightmoveJobs returns jobs whose next attempt is at or before now.
func (s *Store) DueRightmoveJobs(now time.Time) ([]RightmoveJob, error) {
	var out []RightmoveJob
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketRightmoveQueue).ForEach(func(_, v []byte) error {
			var job RightmoveJob
			if err := json.Unmarshal(v, &job); err != nil {
				return err
			}
			if !job.NextAttempt.After(now) {
				out = append(out, job)
			}
			return nil
		})
	})
	return out, err
}

func (s *Store) CountRightmoveJobs() (int, error) {
	var n int
	err := s.db.View(func(tx *bolt.Tx) error {
		n = tx.Bucket(bucketRightmoveQueue).Stats().KeyN
		return nil
	})
	return n, err
}

// FinishRightmoveJob records the outcome of a job. A zero retryAt removes the job;
// otherwise it is rescheduled. If the property was re-queued while the job ran, the
// newer job is kept untouched so the latest changes still get sent.
func (s *Store) FinishRightmoveJob(job RightmoveJob, retryAt time.Time, update func(*models.RightmoveSync)) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		q := tx.Bucket(bucketRightmoveQueue)
		pending := false
		var cur RightmoveJob
		if data := q.Get([]byte(job.PropertyID)); data != nil {
			if err := json.Unmarshal(data, &cur); err != nil {
				return err
			}
			switch {
			case cur.Version != job.Version:
				pending = true
			case retryAt.IsZero():
				if err := q.Delete([]byte(job.PropertyID)); err != nil {
					return err
				}
			default:
				cur.Attempts++
				cur.NextAttempt = retryAt
				if err := put(q, job.PropertyID, cur); err != nil {
					return err
				}
				pending = true
			}
		}
		return updateRightmoveSync(tx, job.PropertyID, func(rs *models.RightmoveSync) {
			if update != nil {
				update(rs)
			}
			rs.Pending = pending
		})
	})
}

// updateRightmoveSync changes only a property's Rightmove status, so it never
// overwrites edits made in admin at the same time. Missing properties are ignored.
func updateRightmoveSync(tx *bolt.Tx, propertyID string, fn func(*models.RightmoveSync)) error {
	b := tx.Bucket(bucketProperties)
	data := b.Get([]byte(propertyID))
	if data == nil {
		return nil
	}
	var p models.Property
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	fn(&p.Rightmove)
	return put(b, p.ID, &p)
}
