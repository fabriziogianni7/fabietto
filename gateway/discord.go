package gateway

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"

	"custom-agent/wallet/redact"
)

// DiscordGateway implements Gateway and Sender for Discord.
type DiscordGateway struct {
	token   string
	session *discordgo.Session
	mu      sync.RWMutex
}

// NewDiscord creates a Discord gateway. Token must be non-empty.
func NewDiscord(token string) *DiscordGateway {
	return &DiscordGateway{token: token}
}

// Send delivers an outbound message to the channel. Implements Sender.
func (g *DiscordGateway) Send(ctx context.Context, platform, userID, chatID, text string) error {
	g.mu.RLock()
	s := g.session
	g.mu.RUnlock()
	if s == nil {
		return fmt.Errorf("discord: session not initialized")
	}
	_, err := s.ChannelMessageSend(chatID, text)
	return err
}

// Run starts the Discord bot and processes messages until ctx is cancelled.
func (g *DiscordGateway) Run(ctx context.Context, handler Handler) error {
	s, err := discordgo.New("Bot " + g.token)
	if err != nil {
		return fmt.Errorf("discord: %w", err)
	}

	g.mu.Lock()
	g.session = s
	g.mu.Unlock()

	s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.Bot {
			return
		}
		if m.Content == "" {
			return
		}

		log.Printf("[discord] [%s] %s", m.Author.Username, redact.Redact(m.Content))

		incoming := IncomingMessage{
			Platform:  "discord",
			UserID:   m.Author.ID,
			ChatID:   m.ChannelID,
			Text:     strings.TrimSpace(m.Content),
			ReplyToID: m.ID,
		}

		reply := handler(incoming)

		ref := &discordgo.MessageReference{MessageID: m.ID, ChannelID: m.ChannelID}
		if _, err := s.ChannelMessageSendReply(m.ChannelID, reply, ref); err != nil {
			if _, err := s.ChannelMessageSend(m.ChannelID, reply); err != nil {
				log.Printf("[discord] send error: %v", err)
			}
		}
	})

	if err := s.Open(); err != nil {
		return fmt.Errorf("discord: %w", err)
	}
	defer s.Close()

	log.Printf("[discord] Connected as %s", s.State.User.Username)

	<-ctx.Done()
	return ctx.Err()
}
