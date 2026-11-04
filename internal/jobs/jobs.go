// Package jobs runs the scheduled agents: the alert engine every 15 minutes,
// escalation timers and callbacks every 5 minutes, the outbox continuously,
// and data-retention clean-up daily. A database lease makes sure only one
// replica runs them.
package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"sync"
	"time"

	"github.com/vivekyarra/bhagirath/internal/agents"
	"github.com/vivekyarra/bhagirath/internal/notify"
)

// Scheduler owns the background loops.
type Scheduler struct {
	Engine     *agents.Engine
	Dispatcher *notify.Dispatcher
	Log        *slog.Logger
	holder     string
	mu         sync.Mutex // serialises RunAll with the loops
	lastRun    map[string]time.Time
}

// New creates a scheduler.
func New(e *agents.Engine, d *notify.Dispatcher, log *slog.Logger) *Scheduler {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return &Scheduler{Engine: e, Dispatcher: d, Log: log, holder: hex.EncodeToString(b), lastRun: map[string]time.Time{}}
}

// Start launches the loops; they stop when ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context, wg *sync.WaitGroup) {
	cfg := s.Engine.Cfg
	s.loop(ctx, wg, "alerts", cfg.AlertInterval, s.alerts)
	s.loop(ctx, wg, "timers", cfg.TimerInterval, s.timers)
	s.loop(ctx, wg, "outbox", cfg.OutboxInterval, s.outbox)
	s.loop(ctx, wg, "retention", 6*time.Hour, s.retention)
}

func (s *Scheduler) loop(ctx context.Context, wg *sync.WaitGroup, name string, every time.Duration, f func(context.Context) error) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			s.runLeased(ctx, name, every, f)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

func (s *Scheduler) runLeased(ctx context.Context, name string, every time.Duration, f func(context.Context) error) {
	ttl := max(3*every, time.Minute)
	ok, err := s.Engine.Store.AcquireLease(ctx, name, s.holder, ttl, time.Now().UTC())
	if err != nil || !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	start := time.Now()
	if err := f(ctx); err != nil && ctx.Err() == nil {
		s.Log.Error("job failed", "job", name, "err", err)
	}
	s.lastRun[name] = time.Now()
	if d := time.Since(start); d > every/2 {
		s.Log.Warn("job slow", "job", name, "duration", d)
	}
}

func (s *Scheduler) alerts(ctx context.Context) error {
	_, err := s.Engine.EvaluateAll(ctx)
	return err
}

func (s *Scheduler) timers(ctx context.Context) error {
	if _, err := s.Engine.ProcessTimeouts(ctx); err != nil {
		return err
	}
	_, err := s.Engine.ProcessCallbacks(ctx)
	return err
}

func (s *Scheduler) outbox(ctx context.Context) error {
	_, err := s.Dispatcher.Run(ctx)
	return err
}

func (s *Scheduler) retention(ctx context.Context) error {
	now := s.Engine.Clock.Now()
	if n, err := s.Engine.Store.PurgeExpiredMedia(ctx, now); err != nil {
		return err
	} else if n > 0 {
		s.Log.Info("retention: audio deleted", "count", n)
	}
	if n, err := s.Engine.Store.PurgeUnconsentedPhones(ctx, now.Add(-s.Engine.Cfg.ConsentWindow)); err != nil {
		return err
	} else if n > 0 {
		s.Log.Info("retention: unconsented numbers purged", "count", n)
	}
	return nil
}

// RunAll runs every job once, in dependency order (used after the demo clock
// is advanced, and by tests).
func (s *Scheduler) RunAll(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range []func(context.Context) error{s.timers, s.alerts, s.timers, s.outbox} {
		if err := f(ctx); err != nil {
			return err
		}
	}
	return nil
}

// LastRun reports when each job last completed on this replica.
func (s *Scheduler) LastRun() map[string]time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]time.Time, len(s.lastRun))
	for k, v := range s.lastRun {
		out[k] = v
	}
	return out
}
