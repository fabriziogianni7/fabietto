package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"custom-agent/agent"
	"custom-agent/config"
	"custom-agent/gateway"
	"custom-agent/tools"

	"github.com/sashabaranov/go-openai"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	personality, err := os.ReadFile("PERSONALITY.md")
	if err != nil {
		log.Fatalf("failed to load PERSONALITY.md: %v", err)
	}
	const toolInstruction = "\n\nYou have access to tools. Use them when they help answer the user's question—for example, read files, run commands, or search the web when needed."
	systemPrompt := strings.TrimSpace(string(personality)) + toolInstruction

	llmConfig := openai.DefaultConfig(cfg.GroqAPIKey)
	llmConfig.BaseURL = "https://api.groq.com/openai/v1"
	llm := openai.NewClientWithConfig(llmConfig)

	toolSet := tools.NewTools(cfg.BraveSearchAPIKey)
	a := agent.New(llm, systemPrompt, cfg.CompactionThreshold, toolSet)

	handler := func(msg gateway.IncomingMessage) string {
		return a.HandleMessage(context.Background(), msg)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start enabled gateways
	type gwStarter struct {
		name string
		gw   gateway.Gateway
	}
	var starters []gwStarter
	if cfg.TelegramBotToken != "" {
		starters = append(starters, gwStarter{"telegram", gateway.NewTelegram(cfg.TelegramBotToken)})
	}
	if cfg.DiscordToken != "" {
		starters = append(starters, gwStarter{"discord", gateway.NewDiscord(cfg.DiscordToken)})
	}
	if cfg.HTTPPort != "" {
		starters = append(starters, gwStarter{"http", gateway.NewHTTP(cfg.HTTPPort)})
	}
	if cfg.SignalCliURL != "" && cfg.SignalNumber != "" {
		starters = append(starters, gwStarter{"signal", gateway.NewSignal(cfg.SignalCliURL, cfg.SignalNumber)})
	}
	for _, s := range starters {
		gw, name := s.gw, s.name
		go func() {
			if err := gw.Run(ctx, handler); err != nil && err != context.Canceled {
				log.Printf("[%s] %v", name, err)
			}
		}()
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down...")
	cancel()
}
