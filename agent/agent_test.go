package agent

import (
	"testing"

	"github.com/sashabaranov/go-openai"
)

func TestConvertToMessagesAPIFormat(t *testing.T) {
	tests := []struct {
		name      string
		in        []openai.ChatCompletionMessage
		wantSys   string
		firstUser string
	}{
		{
			name: "system prepended to first user",
			in: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: "You are helpful."},
				{Role: openai.ChatMessageRoleUser, Content: "Hello"},
			},
			wantSys:   "You are helpful.",
			firstUser: "You are helpful.\n\n---\n\nHello",
		},
		{
			name: "multiple system blocks merged",
			in: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: "Block 1"},
				{Role: openai.ChatMessageRoleSystem, Content: "Block 2"},
				{Role: openai.ChatMessageRoleUser, Content: "Hi"},
			},
			wantSys:   "Block 1\n\nBlock 2",
			firstUser: "Block 1\n\nBlock 2\n\n---\n\nHi",
		},
		{
			name: "no system messages",
			in: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleUser, Content: "Hi"},
			},
			firstUser: "Hi",
		},
		{
			name: "no user message",
			in: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: "Sys"},
			},
			wantSys: "Sys",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertToMessagesAPIFormat(tt.in)
			// Check no system role remains
			for _, m := range got {
				if m.Role == openai.ChatMessageRoleSystem {
					t.Fatal("system role should not appear in output")
				}
			}
			if tt.firstUser != "" {
				for _, m := range got {
					if m.Role == openai.ChatMessageRoleUser {
						if m.Content != tt.firstUser {
							t.Errorf("first user content: got %q, want %q", m.Content, tt.firstUser)
						}
						break
					}
				}
			}
		})
	}
}
