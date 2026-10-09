package web

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AndrewDMcC/estate-agency/internal/models"
	"github.com/AndrewDMcC/estate-agency/internal/rightmove"
	"github.com/AndrewDMcC/estate-agency/internal/store"
)

type rightmoveClient interface {
	Mode() rightmove.Mode
	DryRun() bool
	SendProperty(ctx context.Context, p *models.Property, baseURL string) error
	RemoveProperty(ctx context.Context, agentRef string) error
}

const (
	rightmovePollInterval = 15 * time.Second
	rightmoveRetryBase    = 30 * time.Second
	rightmoveRetryMax     = time.Hour
)

// listedRef is the agent ref the property is currently listed under on Rightmove, if any.
func listedRef(p *models.Property) string {
	if p.Rightmove.ListedRef != "" {
		return p.Rightmove.ListedRef
	}
	if p.Rightmove.Listed {
		return p.AgentRef()
	}
	return ""
}

// queueRightmove schedules a property for sending, or for removal when it is no longer live.
func (s *Server) queueRightmove(p *models.Property) {
	if !p.Live() && listedRef(p) == "" {
		return
	}
	ref := listedRef(p)
	if ref == "" {
		ref = p.AgentRef()
	}
	s.enqueueRightmove(p.ID, ref)
}

func (s *Server) enqueueRightmove(propertyID, agentRef string) {
	if err := s.store.EnqueueRightmove(propertyID, agentRef); err != nil {
		s.log.Error("queue rightmove update", "id", propertyID, "err", err)
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// RunRightmoveWorker sends queued updates until ctx is cancelled.
func (s *Server) RunRightmoveWorker(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-timer.C:
		}
		s.processRightmoveQueue(ctx, time.Now().UTC())
		timer.Reset(rightmovePollInterval)
	}
}

func (s *Server) processRightmoveQueue(ctx context.Context, now time.Time) {
	jobs, err := s.store.DueRightmoveJobs(now)
	if err != nil {
		s.log.Error("load rightmove queue", "err", err)
		return
	}
	for _, job := range jobs {
		if ctx.Err() != nil {
			return
		}
		s.processRightmoveJob(ctx, job, now)
	}
}

func retryDelay(attempts int) time.Duration {
	d := rightmoveRetryBase
	for i := 1; i < attempts && d < rightmoveRetryMax; i++ {
		d *= 2
	}
	return min(d, rightmoveRetryMax)
}

func (s *Server) processRightmoveJob(ctx context.Context, job store.RightmoveJob, now time.Time) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	p, err := s.store.GetProperty(job.PropertyID)
	if errors.Is(err, store.ErrNotFound) {
		err = s.rm.RemoveProperty(ctx, job.AgentRef)
		s.finishRightmoveJob(job, now, err, false, nil)
		return
	}
	if err != nil {
		s.log.Error("load property for rightmove", "id", job.PropertyID, "err", err)
		return
	}

	rs := p.Rightmove
	ref := listedRef(p)
	listed := ref != ""
	permanent := false
	if listed && ref != p.AgentRef() {
		if err = s.rm.RemoveProperty(ctx, ref); err == nil {
			listed, ref = false, ""
		}
	}
	if err == nil {
		switch {
		case p.Live():
			if problems := rightmove.Validate(p); len(problems) > 0 {
				err = fmt.Errorf("not sent: %s", strings.Join(problems, "; "))
				permanent = true
			} else if err = s.rm.SendProperty(ctx, p, s.cfg.BaseURL); err == nil && !s.rm.DryRun() {
				listed, ref = true, p.AgentRef()
			}
		case listed:
			if err = s.rm.RemoveProperty(ctx, ref); err == nil {
				listed, ref = false, ""
			}
		}
	}
	rs.Listed, rs.ListedRef = listed, ref
	s.finishRightmoveJob(job, now, err, permanent, &rs)
}

func (s *Server) finishRightmoveJob(job store.RightmoveJob, now time.Time, err error, permanent bool, rs *models.RightmoveSync) {
	var retryAt time.Time
	if err != nil {
		s.log.Warn("rightmove update failed", "id", job.PropertyID, "attempt", job.Attempts+1, "err", err)
		if !permanent {
			retryAt = now.Add(retryDelay(job.Attempts + 1))
		}
	}
	update := func(cur *models.RightmoveSync) {
		if rs != nil {
			*cur = *rs
		}
		cur.LastSyncAt = now
		cur.DryRun = s.rm.DryRun()
		cur.LastError, cur.Attempts, cur.NextAttemptAt = "", 0, time.Time{}
		if err != nil {
			cur.LastError = err.Error()
		}
		if !retryAt.IsZero() {
			cur.Attempts, cur.NextAttemptAt = job.Attempts+1, retryAt
		}
	}
	if err := s.store.FinishRightmoveJob(job, retryAt, update); err != nil {
		s.log.Error("save rightmove result", "id", job.PropertyID, "err", err)
	}
}
