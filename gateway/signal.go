package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"custom-agent/wallet/redact"
)

// SignalGateway implements Gateway for Signal via signal-cli-rest-api.
// Requires signal-cli-rest-api running (e.g. docker run -p 8080:8080 bbernhard/signal-cli-rest-api).
type SignalGateway struct {
	baseURL string // e.g. http://localhost:8080
	number  string // bot's Signal number, e.g. +1234567890
}

// NewSignal creates a Signal gateway.
// baseURL is the signal-cli-rest-api URL (e.g. http://localhost:8080).
// number is the registered bot number (e.g. +1234567890).
func NewSignal(baseURL, number string) *SignalGateway {
	return &SignalGateway{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		number:  number,
	}
}

// receiveResponse matches signal-cli-rest-api receive endpoint response.
type receiveResponse struct {
	Results []struct {
		Envelope struct {
			Source       string `json:"source"`
			SourceNumber string `json:"sourceNumber"`
			SourceUUID   string `json:"sourceUuid"`
			Timestamp    int64  `json:"timestamp"`
			DataMessage  *struct {
				Message string `json:"message"`
			} `json:"dataMessage"`
			SyncMessage *struct {
				SentMessage *struct {
					Message string `json:"message"`
					Dest    string `json:"destination"`
				} `json:"sentMessage"`
			} `json:"syncMessage"`
		} `json:"envelope"`
	} `json:"results"`
}

// Run polls signal-cli-rest-api for messages and processes them until ctx is cancelled.
func (g *SignalGateway) Run(ctx context.Context, handler Handler) error {
	receiveURL := g.baseURL + "/v2/receive/" + url.PathEscape(g.number)
	client := &http.Client{Timeout: 30 * time.Second}

	log.Printf("[signal] Listening for messages on %s", g.number)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			req, err := http.NewRequestWithContext(ctx, "GET", receiveURL, nil)
			if err != nil {
				return fmt.Errorf("signal: %w", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				log.Printf("[signal] receive error: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}

			var data receiveResponse
			if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
				resp.Body.Close()
				log.Printf("[signal] decode error: %v", err)
				continue
			}
			resp.Body.Close()

			for _, r := range data.Results {
				env := r.Envelope
				var text string
				var sender string

				if env.DataMessage != nil {
					text = strings.TrimSpace(env.DataMessage.Message)
					sender = env.Source
					if sender == "" {
						sender = env.SourceNumber
					}
				} else if env.SyncMessage != nil && env.SyncMessage.SentMessage != nil {
					// Sync message from our own device, skip
					continue
				} else {
					continue
				}

				if text == "" {
					continue
				}

				log.Printf("[signal] [%s] %s", sender, redact.Redact(text))

				incoming := IncomingMessage{
					Platform:  "signal",
					UserID:   sender,
					ChatID:   sender,
					Text:     text,
					ReplyToID: fmt.Sprintf("%d", env.Timestamp),
				}

				reply := handler(incoming)

				if err := g.send(ctx, sender, reply); err != nil {
					log.Printf("[signal] send error: %v", err)
				}
			}

			if len(data.Results) == 0 {
				time.Sleep(2 * time.Second)
			}
		}
	}
}

// Send delivers an outbound message to the recipient. Implements Sender.
// For Signal, chatID and userID are typically the recipient number.
func (g *SignalGateway) Send(ctx context.Context, platform, userID, chatID, text string) error {
	recipient := chatID
	if recipient == "" {
		recipient = userID
	}
	return g.send(ctx, recipient, text)
}

func (g *SignalGateway) send(ctx context.Context, recipient, message string) error {
	body := map[string]interface{}{
		"number":     g.number,
		"recipients": []string{recipient},
		"message":    message,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", g.baseURL+"/v2/send", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}
