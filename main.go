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

	a := agent.New(llm, systemPrompt, cfg.CompactionThreshold)

	handler := func(msg gateway.IncomingMessage) string {
		return a.HandleMessage(context.Background(), msg)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start enabled gateways
	if cfg.TelegramBotToken != "" {
		go func() {
			tg := gateway.NewTelegram(cfg.TelegramBotToken)
			if err := tg.Run(ctx, handler); err != nil && err != context.Canceled {
				log.Printf("telegram gateway: %v", err)
			}
		}()
	}
	if cfg.DiscordToken != "" {
		go func() {
			dg := gateway.NewDiscord(cfg.DiscordToken)
			if err := dg.Run(ctx, handler); err != nil && err != context.Canceled {
				log.Printf("discord gateway: %v", err)
			}
		}()
	}
	if cfg.HTTPPort != "" {
		go func() {
			hg := gateway.NewHTTP(cfg.HTTPPort)
			if err := hg.Run(ctx, handler); err != nil && err != context.Canceled {
				log.Printf("http gateway: %v", err)
			}
		}()
	}
	if cfg.SignalCliURL != "" && cfg.SignalNumber != "" {
		go func() {
			sg := gateway.NewSignal(cfg.SignalCliURL, cfg.SignalNumber)
			if err := sg.Run(ctx, handler); err != nil && err != context.Canceled {
				log.Printf("signal gateway: %v", err)
			}
		}()
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down...")
	cancel()
}
