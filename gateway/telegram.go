package gateway

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TelegramGateway implements Gateway and Sender for Telegram.
type TelegramGateway struct {
	token string
	bot   *tgbotapi.BotAPI
	mu    sync.RWMutex
}

// NewTelegram creates a Telegram gateway. Token must be non-empty.
func NewTelegram(token string) *TelegramGateway {
	return &TelegramGateway{token: token}
}

// Send delivers an outbound message to the chat. Implements Sender.
func (g *TelegramGateway) Send(ctx context.Context, platform, userID, chatID, text string) error {
	g.mu.RLock()
	bot := g.bot
	g.mu.RUnlock()
	if bot == nil {
		return fmt.Errorf("telegram: bot not initialized")
	}
	cid, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return fmt.Errorf("telegram: invalid chat_id %q: %w", chatID, err)
	}
	msg := tgbotapi.NewMessage(cid, text)
	_, err = bot.Send(msg)
	return err
}

// Run starts the Telegram bot and processes messages until ctx is cancelled.
func (g *TelegramGateway) Run(ctx context.Context, handler Handler) error {
	bot, err := tgbotapi.NewBotAPI(g.token)
	if err != nil {
		return fmt.Errorf("telegram: %w", err)
	}

	g.mu.Lock()
	g.bot = bot
	g.mu.Unlock()

	bot.Debug = false
	log.Printf("[telegram] Authorized as @%s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update := <-updates:
			if update.Message == nil || update.Message.From == nil {
				continue
			}

			msg := update.Message
			log.Printf("[telegram] [%s] %s", msg.From.UserName, msg.Text)

			incoming := IncomingMessage{
				Platform:  "telegram",
				UserID:   fmt.Sprintf("%d", msg.From.ID),
				ChatID:   fmt.Sprintf("%d", msg.Chat.ID),
				Text:     msg.Text,
				ReplyToID: fmt.Sprintf("%d", msg.MessageID),
			}

			reply := handler(incoming)

			response := tgbotapi.NewMessage(msg.Chat.ID, reply)
			response.ReplyToMessageID = msg.MessageID
			if _, err := bot.Send(response); err != nil {
				log.Printf("[telegram] send error: %v", err)
			}
		}
	}
}
