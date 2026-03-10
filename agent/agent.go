package agent

import (
	"context"
	"strings"

	"custom-agent/compaction"
	"custom-agent/gateway"
	"custom-agent/session"
	"custom-agent/tools"

	"github.com/sashabaranov/go-openai"
)

const (
	groqModel     = "llama-3.1-8b-instant"
	maxToolRounds = 10
)

// Agent processes messages and returns replies using an LLM.
type Agent struct {
	client       *openai.Client
	systemPrompt string
	compactor    *compaction.Compactor
}

// New creates an Agent with the given LLM client and system prompt.
// tokenThreshold: when context exceeds this (approx tokens), compaction is triggered. 0 = default (4000).
func New(client *openai.Client, systemPrompt string, tokenThreshold int) *Agent {
	return &Agent{
		client:       client,
		systemPrompt: systemPrompt,
		compactor:    compaction.NewCompactor(client, groqModel, tokenThreshold),
	}
}

// HandleMessage processes an incoming message and returns the reply.
func (a *Agent) HandleMessage(ctx context.Context, msg gateway.IncomingMessage) string {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return "Hello! Send me a message and I'll respond."
	}

	// Handle approval messages
	if cmd, ok := tools.ParseApprovalMessage(text); ok {
		if err := tools.ApproveCommand(cmd); err != nil {
			return "Approval failed: " + err.Error()
		}
		return "Approved."
	}

	history, err := session.Load(msg.Platform, msg.UserID)
	if err != nil {
		// log handled by caller if needed
	}

	// Threshold-based compaction: summarize old context when it exceeds limit
	summaryBlock, recent, _ := a.compactor.CompactIfNeeded(ctx, history, a.systemPrompt)
	if recent == nil {
		recent = session.Recent(history)
	}

	messages := make([]openai.ChatCompletionMessage, 0, len(recent)+3)
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: a.systemPrompt,
	})
	if summaryBlock != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: summaryBlock,
		})
	}
	for _, m := range recent {
		role := openai.ChatMessageRoleUser
		if m.Role == "assistant" {
			role = openai.ChatMessageRoleAssistant
		}
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    role,
			Content: m.Content,
		})
	}
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: text,
	})

	toolDefs := tools.Definitions()

	for i := 0; i < maxToolRounds; i++ {
		resp, err := a.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model:    groqModel,
			Messages: messages,
			Tools:    toolDefs,
		})
		if err != nil {
			return "Sorry, I couldn't process that. Please try again."
		}

		if len(resp.Choices) == 0 {
			return "I didn't get a response. Try again?"
		}

		msgResp := resp.Choices[0].Message

		// Execute tool calls (structured)
		if len(msgResp.ToolCalls) > 0 {
			messages = append(messages, msgResp)
			for _, tc := range msgResp.ToolCalls {
				result, err := tools.ExecuteTool(tc.Function.Name, tc.Function.Arguments)
				if err != nil {
					result = "Error: " + err.Error()
				}
				messages = append(messages, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    result,
					ToolCallID: tc.ID,
				})
			}
			continue
		}

		// Fallback: model returned tool format as text
		if toolName, toolArgs, ok := tools.ParseToolCallFromContent(msgResp.Content); ok {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: msgResp.Content,
			})
			result, err := tools.ExecuteTool(toolName, toolArgs)
			if err != nil {
				result = "Error: " + err.Error()
			}
			messages = append(messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    result,
				ToolCallID: "fallback",
				Name:       toolName,
			})
			continue
		}

		// Final text response
		reply := strings.TrimSpace(msgResp.Content)
		if reply == "" {
			return "I didn't get a response. Try again?"
		}
		_ = session.Append(msg.Platform, msg.UserID, text, reply)
		return reply
	}

	return "I hit the tool limit. Please try a simpler request."
}
