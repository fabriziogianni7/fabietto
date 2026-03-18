package opportunity

import (
	"context"
	"fmt"
	"log"
	"time"

	"custom-agent/gateway"

	"github.com/robfig/cron/v3"
)

const scanPrompt = `Scan for opportunities. 1) Use x402_get_stats for inference spend and runway. 2) Use wallet_get_portfolio_value (chain_id 8453 for Base USDC) for holdings and runway. 3) Use wallet_get_activity for recent inflows/outflows. 4) Check market data via Tokenaru (http_request to https://tokenaru.vercel.app/api/lookup). 5) Use wallet_simulate_transaction before executing. 6) Execute if you identify a clear opportunity. Report what you found and did.`

// Runner periodically invokes the agent to scan for opportunities.
type Runner struct {
	handler  func(gateway.IncomingMessage) string
	senders  *gateway.SenderRegistry
	platform string
	userID   string
	chatID   string
	interval time.Duration
}

// NewRunner creates an opportunity scan runner.
// platform, userID, chatID: where to send the agent's reply and wallet approval requests.
// interval: how often to run (e.g. 15*time.Minute). Must be > 0.
func NewRunner(handler func(gateway.IncomingMessage) string, senders *gateway.SenderRegistry, platform, userID, chatID string, interval time.Duration) *Runner {
	return &Runner{
		handler:  handler,
		senders:  senders,
		platform: platform,
		userID:   userID,
		chatID:   chatID,
		interval: interval,
	}
}

// Start begins the cron loop. Blocks until ctx is cancelled.
func (r *Runner) Start(ctx context.Context) {
	if r.interval <= 0 {
		return
	}
	c := cron.New()
	mins := int(r.interval.Minutes())
	if mins < 1 {
		mins = 1
	}
	spec := fmt.Sprintf("@every %dm", mins)
	_, err := c.AddFunc(spec, func() {
		r.runScan(ctx)
	})
	if err != nil {
		log.Printf("[opportunity] cron add func: %v", err)
		return
	}
	c.Start()
	defer c.Stop()

	log.Printf("[opportunity] cron started, scanning every %s → %s/%s", r.interval, r.platform, r.userID)
	<-ctx.Done()
}

func (r *Runner) runScan(ctx context.Context) {
	msg := gateway.IncomingMessage{
		Platform: r.platform,
		UserID:   r.userID,
		ChatID:   r.chatID,
		Text:     scanPrompt,
	}
	reply := r.handler(msg)
	if reply == "" {
		return
	}
	if err := r.senders.Send(ctx, r.platform, r.userID, r.chatID, reply); err != nil {
		log.Printf("[opportunity] send reply: %v", err)
		return
	}
	log.Printf("[opportunity] scan complete, reply sent to %s/%s", r.platform, r.userID)
}
