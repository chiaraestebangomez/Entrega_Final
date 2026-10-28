package ingestion

import (
	"context"
	"log/slog"
	"time"

	"trotamundos/internal/domain"
)

type Worker struct {
	svc        *Service
	repo       Repository
	log        *slog.Logger
	poll       time.Duration
	staleAfter time.Duration
	maxRetries int
}

func NewWorker(svc *Service, repo Repository, log *slog.Logger, poll, staleAfter time.Duration) *Worker {
	if poll <= 0 {
		poll = 2 * time.Second
	}
	if staleAfter <= 0 {
		staleAfter = 2 * time.Minute
	}
	return &Worker{svc: svc, repo: repo, log: log, poll: poll, staleAfter: staleAfter, maxRetries: 5}
}

func (w *Worker) Run(ctx context.Context) {
	w.log.Info("ingestion worker started", "poll", w.poll.String())
	ticker := time.NewTicker(w.poll)
	defer ticker.Stop()
	w.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			w.log.Info("ingestion worker stopped")
			return
		case <-ticker.C:
			w.tick(ctx)
		case <-w.svc.Queue():
			w.tick(ctx)
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	if err := w.repo.RecoverStale(ctx, time.Now().UTC().Add(-w.staleAfter)); err != nil {
		w.log.Error("recover stale ingestion events", "error", err)
	}
	events, err := w.repo.ClaimEvents(ctx, 10)
	if err != nil {
		w.log.Error("claim ingestion events", "error", err)
		return
	}
	for _, ev := range events {
		if err := w.svc.ProcessEvent(ctx, ev); err != nil {
			status := domain.EventPending
			next := time.Now().UTC().Add(time.Duration(1<<min(ev.Attempts, 6)) * time.Second)
			if ev.Attempts >= w.maxRetries {
				status = domain.EventError
			}
			_ = w.repo.MarkError(ctx, ev.ID, status, next, err.Error())
			w.log.Warn("ingestion event failed", "event_id", ev.ID, "attempts", ev.Attempts, "status", status, "error", err)
			continue
		}
		w.log.Info("ingestion event processed", "event_id", ev.ID, "message_id", ev.MessageID)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
