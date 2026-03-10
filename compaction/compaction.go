package compaction

import (
	"context"
	"strings"

	"custom-agent/session"

	"github.com/sashabaranov/go-openai"
)

const (
	// DefaultTokenThreshold triggers compaction when context exceeds this (approx).
	// Llama 8B has 128K context; reserve room for system, tools, response.
	DefaultTokenThreshold = 4000
	// CharsPerToken rough estimate for Latin text
	charsPerToken = 4
)

// Compactor performs threshold-based compaction with structured summarization.
type Compactor struct {
	client    *openai.Client
	model     string
	threshold int // token count; compact when exceeded
}

// NewCompactor creates a compactor.
func NewCompactor(client *openai.Client, model string, tokenThreshold int) *Compactor {
	if tokenThreshold <= 0 {
		tokenThreshold = DefaultTokenThreshold
	}
	return &Compactor{
		client:    client,
		model:     model,
		threshold: tokenThreshold,
	}
}

// EstimateTokens returns approximate token count for messages (chars/4).
func EstimateTokens(msgs []session.Message) int {
	var n int
	for _, m := range msgs {
		n += len(m.Content) / charsPerToken
	}
	return n
}

// CompactIfNeeded returns messages fit for the context window.
// If history exceeds threshold: summarizes old messages into CompactedContext,
// returns [summary_block, recent_messages]. Otherwise returns full history.
func (c *Compactor) CompactIfNeeded(ctx context.Context, history []session.Message, systemPrompt string) (summaryBlock string, recent []session.Message, err error) {
	if len(history) == 0 {
		return "", nil, nil
	}

	totalTokens := EstimateTokens(history)
	if totalTokens <= c.threshold {
		return "", history, nil
	}

	// Find split: keep recent messages under threshold
	recentTokens := 0
	split := len(history)
	for i := len(history) - 1; i >= 0; i-- {
		tok := len(history[i].Content) / charsPerToken
		if recentTokens+tok > c.threshold/2 {
			split = i
			break
		}
		recentTokens += tok
	}

	oldMessages := history[:split]
	recent = history[split:]

	// Summarize old messages into structured format
	summary, err := c.summarize(ctx, oldMessages)
	if err != nil {
		// Fallback: truncate instead of summarize
		return "", session.Recent(history), nil
	}

	block, err := summary.ToPromptBlock()
	if err != nil {
		return "", session.Recent(history), nil
	}

	return block, recent, nil
}

func (c *Compactor) summarize(ctx context.Context, msgs []session.Message) (*CompactedContext, error) {
	var conv strings.Builder
	for _, m := range msgs {
		conv.WriteString(m.Role + ": " + m.Content + "\n")
	}

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: SummarizationPrompt()},
			{Role: openai.ChatMessageRoleUser, Content: "Summarize this conversation:\n\n" + conv.String()},
		},
	})
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return nil, nil
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	return ParseCompactedContext(extractJSON(content))
}

// extractJSON pulls the first {...} from content (handles markdown wrappers).
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return "{}"
	}
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return s[start:]
}
