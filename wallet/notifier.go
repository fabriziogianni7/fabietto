package wallet

import (
	"context"

	"custom-agent/gateway"
)

// SenderNotifier implements ApprovalNotifier using gateway.SenderRegistry.
type SenderNotifier struct {
	senders *gateway.SenderRegistry
}

// NewSenderNotifier creates a notifier that sends via the given registry.
func NewSenderNotifier(senders *gateway.SenderRegistry) *SenderNotifier {
	return &SenderNotifier{senders: senders}
}

// Notify sends the message to the guardian via the appropriate gateway.
func (n *SenderNotifier) Notify(ctx context.Context, platform, userID, chatID, message string) error {
	return n.senders.Send(ctx, platform, userID, chatID, message)
}
