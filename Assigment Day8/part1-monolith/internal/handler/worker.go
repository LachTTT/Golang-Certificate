package handler

import (
	"context"
	"log"
	"time"
)

// StartWorker runs the background jobs until ctx is cancelled:
//  1. mark payment sessions past their 10-minute window as 'expired'
//  2. re-drive transfers stuck in 'pending' (e.g. after a crash)
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
		`UPDATE transactions SET status = 'expired', completed_at = now()
		 WHERE type = 'payment' AND status = 'pending' AND expires_at <= now()`)
	if err != nil {
		log.Printf("sweep payments: %v", err)
	} else if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("expired %d payment session(s)", n)
	}

	rows, err := h.DB.QueryContext(ctx,
		`SELECT id::text FROM transactions
		 WHERE type = 'transfer' AND status = 'pending' AND created_at < now() - interval '30 seconds'
		 LIMIT 100`)
	if err != nil {
		log.Printf("sweep transfers: %v", err)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		h.processTransfer(id)
	}
}
