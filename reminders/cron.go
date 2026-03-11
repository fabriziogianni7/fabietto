package reminders

import (
	"context"
	"log"
	"time"

	"custom-agent/gateway"

	"github.com/robfig/cron/v3"
)

// Runner executes scheduled reminders via the SenderRegistry.
type Runner struct {
	store   *Store
	senders *gateway.SenderRegistry
}

// NewRunner creates a cron runner for reminders.
func NewRunner(store *Store, senders *gateway.SenderRegistry) *Runner {
	return &Runner{store: store, senders: senders}
}

// Start begins the cron loop. It runs every minute and sends due reminders.
// Blocks until ctx is cancelled.
func (r *Runner) Start(ctx context.Context) {
	c := cron.New()
	_, err := c.AddFunc("@every 1m", func() {
		r.runDueReminders(ctx)
	})
	if err != nil {
		log.Printf("[reminders] cron add func: %v", err)
		return
	}
	c.Start()
	defer c.Stop()

	log.Printf("[reminders] cron started, checking every minute")
	<-ctx.Done()
}

func (r *Runner) runDueReminders(ctx context.Context) {
	list, err := r.store.List()
	if err != nil {
		log.Printf("[reminders] list: %v", err)
		return
	}

	// Parser supports both 5-field (min hour day month dow) and 6-field (sec min hour day month dow)
	parser := cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

	now := time.Now()
	truncated := now.Truncate(time.Minute)
	checkTime := truncated.Add(-time.Second)

	for _, rem := range list {
		sched, err := parser.Parse(rem.Schedule)
		if err != nil {
			log.Printf("[reminders] invalid schedule %q for %s: %v", rem.Schedule, rem.ID, err)
			continue
		}
		next := sched.Next(checkTime)
		if !next.Equal(truncated) {
			continue
		}

		if err := r.senders.Send(ctx, rem.Platform, rem.UserID, rem.ChatID, rem.Message); err != nil {
			log.Printf("[reminders] send %s: %v", rem.ID, err)
			continue
		}
		log.Printf("[reminders] sent %s to %s/%s", rem.ID, rem.Platform, rem.UserID)
	}
}
