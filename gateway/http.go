package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// HTTPGateway implements Gateway for HTTP POST /chat.
type HTTPGateway struct {
	port string
}

// NewHTTP creates an HTTP gateway. Port is e.g. "5000".
func NewHTTP(port string) *HTTPGateway {
	return &HTTPGateway{port: port}
}

// Run starts the HTTP server and processes requests until ctx is cancelled.
func (g *HTTPGateway) Run(ctx context.Context, handler Handler) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			UserID  string `json:"user_id"`
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		if req.UserID == "" || strings.TrimSpace(req.Message) == "" {
			http.Error(w, "user_id and message required", http.StatusBadRequest)
			return
		}

		incoming := IncomingMessage{
			Platform: "http",
			UserID:  req.UserID,
			ChatID:  req.UserID, // echo back to same "chat"
			Text:   strings.TrimSpace(req.Message),
		}

		reply := handler(incoming)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"response": reply})
	})

	server := &http.Server{Addr: ":" + g.port, Handler: mux}

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	log.Printf("[http] Listening on :%s", g.port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http: %w", err)
	}
	return ctx.Err()
}
