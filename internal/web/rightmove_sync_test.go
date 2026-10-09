package web

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/AndrewDMcC/estate-agency/internal/models"
	"github.com/AndrewDMcC/estate-agency/internal/rightmove"
	"github.com/AndrewDMcC/estate-agency/internal/store"
)

type fakeRightmove struct {
	mu      sync.Mutex
	fail    int
	sends   []string
	removes []string
}

func (f *fakeRightmove) Mode() rightmove.Mode { return rightmove.ModeTest }
func (f *fakeRightmove) DryRun() bool         { return false }

func (f *fakeRightmove) failing() error {
	if f.fail > 0 {
		f.fail--
		return errors.New("rightmove unavailable")
	}
	return nil
}

func (f *fakeRightmove) SendProperty(_ context.Context, p *models.Property, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failing(); err != nil {
		return err
	}
	f.sends = append(f.sends, p.AgentRef())
	return nil
}

func (f *fakeRightmove) RemoveProperty(_ context.Context, ref string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failing(); err != nil {
		return err
	}
	f.removes = append(f.removes, ref)
	return nil
}

func newQueueServer(t *testing.T) (*Server, *store.Store, *fakeRightmove, *models.Property) {
	t.Helper()
	srv, st := newTestServer(t)
	fake := &fakeRightmove{}
	srv.rm = fake
	props, err := st.ListProperties(func(p *models.Property) bool { return p.Live() && len(rightmove.Validate(p)) == 0 })
	if err != nil || len(props) == 0 {
		t.Fatalf("need a valid live demo property: %v", err)
	}
	return srv, st, fake, props[0]
}

func reload(t *testing.T, st *store.Store, id string) *models.Property {
	t.Helper()
	p, err := st.GetProperty(id)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRightmoveQueueSendsAndClears(t *testing.T) {
	srv, st, fake, p := newQueueServer(t)
	srv.queueRightmove(p)
	if !reload(t, st, p.ID).Rightmove.Pending {
		t.Fatal("property should be marked pending")
	}
	srv.processRightmoveQueue(context.Background(), time.Now().UTC())
	got := reload(t, st, p.ID).Rightmove
	if !slices.Equal(fake.sends, []string{p.AgentRef()}) || !got.Listed || got.Pending || got.LastError != "" || got.ListedRef != p.AgentRef() {
		t.Fatalf("sends=%v status=%+v", fake.sends, got)
	}
	if n, _ := st.CountRightmoveJobs(); n != 0 {
		t.Errorf("queue should be empty, has %d", n)
	}
}

func TestRightmoveQueueRetriesWithBackoff(t *testing.T) {
	srv, st, fake, p := newQueueServer(t)
	fake.fail = 1
	srv.queueRightmove(p)
	now := time.Now().UTC()
	srv.processRightmoveQueue(context.Background(), now)
	got := reload(t, st, p.ID).Rightmove
	if !got.Pending || got.LastError == "" || got.Attempts != 1 || !got.NextAttemptAt.Equal(now.Add(30*time.Second)) {
		t.Fatalf("unexpected status after failure: %+v", got)
	}
	srv.processRightmoveQueue(context.Background(), now.Add(10*time.Second))
	if len(fake.sends) != 0 {
		t.Fatal("should not retry before the backoff has passed")
	}
	srv.processRightmoveQueue(context.Background(), now.Add(time.Minute))
	got = reload(t, st, p.ID).Rightmove
	if len(fake.sends) != 1 || got.Pending || got.LastError != "" || got.Attempts != 0 {
		t.Fatalf("retry should succeed: sends=%v status=%+v", fake.sends, got)
	}
}

func TestRightmoveQueueRemovesDeletedAndSold(t *testing.T) {
	srv, st, fake, p := newQueueServer(t)
	srv.queueRightmove(p)
	srv.processRightmoveQueue(context.Background(), time.Now().UTC())

	p = reload(t, st, p.ID)
	p.Status = models.StatusSold
	if err := st.SaveProperty(p); err != nil {
		t.Fatal(err)
	}
	srv.queueRightmove(p)
	srv.processRightmoveQueue(context.Background(), time.Now().UTC())
	if !slices.Equal(fake.removes, []string{p.AgentRef()}) || reload(t, st, p.ID).Rightmove.Listed {
		t.Fatalf("sold property should be removed: %v", fake.removes)
	}

	p.Status = models.StatusAvailable
	_ = st.SaveProperty(p)
	srv.queueRightmove(p)
	srv.processRightmoveQueue(context.Background(), time.Now().UTC())
	p = reload(t, st, p.ID)
	if err := st.DeleteProperty(p.ID); err != nil {
		t.Fatal(err)
	}
	srv.enqueueRightmove(p.ID, listedRef(p))
	srv.processRightmoveQueue(context.Background(), time.Now().UTC())
	if len(fake.removes) != 2 || fake.removes[1] != p.AgentRef() {
		t.Fatalf("deleted property should be removed: %v", fake.removes)
	}
}

func TestRightmoveQueueRemovesOldRefWhenRefChanges(t *testing.T) {
	srv, st, fake, p := newQueueServer(t)
	srv.queueRightmove(p)
	srv.processRightmoveQueue(context.Background(), time.Now().UTC())
	old := p.AgentRef()
	p = reload(t, st, p.ID)
	p.Ref = "NEW-REF-1"
	_ = st.SaveProperty(p)
	srv.queueRightmove(p)
	srv.processRightmoveQueue(context.Background(), time.Now().UTC())
	if !slices.Equal(fake.removes, []string{old}) || fake.sends[len(fake.sends)-1] != "NEW-REF-1" {
		t.Fatalf("removes=%v sends=%v", fake.removes, fake.sends)
	}
}

func TestRightmoveQueueKeepsNewerJob(t *testing.T) {
	_, st, _, p := newQueueServer(t)
	_ = st.EnqueueRightmove(p.ID, p.AgentRef())
	jobs, _ := st.DueRightmoveJobs(time.Now().UTC())
	_ = st.EnqueueRightmove(p.ID, p.AgentRef())
	if err := st.FinishRightmoveJob(jobs[0], time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
	if n, _ := st.CountRightmoveJobs(); n != 1 || !reload(t, st, p.ID).Rightmove.Pending {
		t.Fatal("job queued during processing must be kept")
	}
}

func TestRightmoveQueueInvalidPropertyNotRetried(t *testing.T) {
	srv, st, fake, p := newQueueServer(t)
	p.Photos = nil
	_ = st.SaveProperty(p)
	srv.queueRightmove(p)
	srv.processRightmoveQueue(context.Background(), time.Now().UTC())
	got := reload(t, st, p.ID).Rightmove
	if len(fake.sends) != 0 || got.Pending || got.LastError == "" {
		t.Fatalf("status=%+v", got)
	}
	if n, _ := st.CountRightmoveJobs(); n != 0 {
		t.Error("invalid property should not stay queued")
	}
}

func TestRetryDelay(t *testing.T) {
	for attempts, want := range map[int]time.Duration{1: 30 * time.Second, 2: time.Minute, 4: 4 * time.Minute, 20: time.Hour} {
		if got := retryDelay(attempts); got != want {
			t.Errorf("retryDelay(%d) = %v, want %v", attempts, got, want)
		}
	}
}
