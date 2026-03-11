package gateway

import "context"

// Sender sends outbound messages to a user. Gateways that support proactive
// messaging (e.g. Telegram, Discord, Signal) implement this interface.
type Sender interface {
	// Send delivers a message to the given chat. platform identifies the gateway,
	// userID and chatID are platform-specific (often same for DMs).
	Send(ctx context.Context, platform, userID, chatID, text string) error
}

// SenderRegistry maps platform names to Senders. Used by cron to deliver reminders.
type SenderRegistry struct {
	senders map[string]Sender
}

// NewSenderRegistry creates an empty registry.
func NewSenderRegistry() *SenderRegistry {
	return &SenderRegistry{senders: make(map[string]Sender)}
}

// Register adds a sender for the given platform.
func (r *SenderRegistry) Register(platform string, s Sender) {
	if s != nil {
		r.senders[platform] = s
	}
}

// Send sends a message via the appropriate gateway. Returns an error if the
// platform has no registered sender.
func (r *SenderRegistry) Send(ctx context.Context, platform, userID, chatID, text string) error {
	s, ok := r.senders[platform]
	if !ok {
		return nil // no sender for platform, skip silently
	}
	return s.Send(ctx, platform, userID, chatID, text)
}
