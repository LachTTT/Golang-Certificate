package handler

import (
	"context"
	"log"
	"time"
)

// StartWorker periodically marks sessions past their window as 'expired'.
func (h *Handler) StartWorker(ctx context.Context, interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		h.sweep(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				h.sweep(ctx)
			}
		}
	}()
}

func (h *Handler) sweep(ctx context.Context) {
	res, err := h.DB.ExecContext(ctx,
		`UPDATE payment_sessions SET status = 'expired', completed_at = now()
		 WHERE status = 'pending' AND expires_at <= now()`)
	if err != nil {
		log.Printf("sweep sessions: %v", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("expired %d payment session(s)", n)
	}
}
