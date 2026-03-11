package agent

import (
	"context"
	"encoding/json"
	"log"
	"regexp"
	"strings"

	"custom-agent/compaction"
	"custom-agent/conversation"
	"custom-agent/gateway"
	"custom-agent/memory"
	"custom-agent/session"
	"custom-agent/tools"

	"github.com/sashabaranov/go-openai"
)

const (
	parentModel   = "moonshotai/kimi-k2-instruct-0905"
	maxToolRounds = 10
)

// subagentModels are rotated per sub-agent index to spread load across Groq's per-model TPM quotas.
var subagentModels = []string{
	"llama-3.1-8b-instant",
	"meta-llama/llama-prompt-guard-2-86m",
	"meta-llama/llama-prompt-guard-2-86m",
}

func subagentModelForIndex(idx int) string {
	if len(subagentModels) == 0 {
		return "llama-3.1-8b-instant"
	}
	return subagentModels[idx%len(subagentModels)]
}

// Agent processes messages and returns replies using an LLM.
type Agent struct {
	client       *openai.Client
	systemPrompt string
	compactor    *compaction.Compactor
	tools        *tools.Tools
	memoryStore  *memory.Store
	convStore    *conversation.Store
}

// New creates an Agent with the given LLM client, system prompt, tools, and optional stores.
// tokenThreshold: when context exceeds this (approx tokens), compaction is triggered. 0 = default (4000).
func New(client *openai.Client, systemPrompt string, tokenThreshold int, toolSet *tools.Tools, convStore *conversation.Store) *Agent {
	return &Agent{
		client:       client,
		systemPrompt: systemPrompt,
		compactor:    compaction.NewCompactor(client, parentModel, tokenThreshold),
		tools:        toolSet,
		memoryStore:  toolSet.MemoryStore,
		convStore:    convStore,
	}
}

// HandleMessage processes an incoming message and returns the reply.
func (a *Agent) HandleMessage(ctx context.Context, msg gateway.IncomingMessage) string {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return "Hello! Send me a message and I'll respond."
	}

	// Handle /new command
	if text == "/new" {
		if err := session.Clear(msg.Platform, msg.UserID); err != nil {
			return "Failed to clear session: " + err.Error()
		}
		_ = conversation.Clear(msg.Platform, msg.UserID)
		return "Session cleared. Starting fresh!"
	}

	// Proactive save when user explicitly says "remember" or "memorize"
	if a.memoryStore != nil {
		if content := extractRememberContent(text); content != "" {
			if err := a.memoryStore.Save(msg.Platform, msg.UserID, content, ""); err != nil {
				log.Printf("[agent] proactive save_memory failed: %v", err)
			} else {
				log.Printf("[agent] proactively saved memory: %s", content)
			}
		}
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

	// Retrieve relevant long-term memories and past conversation (embedding-based if available)
	var contextBlocks []string
	if a.memoryStore != nil {
		if mems, err := a.memoryStore.Search(msg.Platform, msg.UserID, text, 5); err == nil && len(mems) > 0 {
			var b strings.Builder
			b.WriteString("--- Relevant memories ---\n")
			for _, m := range mems {
				b.WriteString("- ")
				b.WriteString(m.Content)
				if m.Tags != "" {
					b.WriteString(" [")
					b.WriteString(m.Tags)
					b.WriteString("]")
				}
				b.WriteString("\n")
			}
			b.WriteString("--- End memories ---")
			contextBlocks = append(contextBlocks, b.String())
		}
	}
	if a.convStore != nil {
		if entries, err := a.convStore.Search(msg.Platform, msg.UserID, text, 5); err == nil && len(entries) > 0 {
			contextBlocks = append(contextBlocks, conversation.FormatEntries(entries))
		}
	}

	messages := make([]openai.ChatCompletionMessage, 0, len(recent)+5)
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: a.systemPrompt,
	})
	for _, block := range contextBlocks {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: block,
		})
	}
	if summaryBlock != "" {
		log.Printf("[agent] context compacted: %d history messages → summary + %d recent", len(history), len(recent))
		log.Printf("[agent] compacted summary:\n%s", summaryBlock)
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
			Model:    parentModel,
			Messages: messages,
			Tools:    toolDefs,
		})
		if err != nil {
			log.Printf("[agent] LLM error: %v", err)
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
				args := tc.Function.Arguments
				var result string
				if tc.Function.Name == "spawn_subagents" {
					result = a.handleSpawnSubagents(ctx, args, msg)
				} else {
					if tc.Function.Name == "save_memory" || tc.Function.Name == "read_memory" {
						if injected, err := tools.InjectMemoryArgs(args, msg.Platform, msg.UserID); err == nil {
							args = injected
						}
					}
					if tc.Function.Name == "create_scheduled_reminder" || tc.Function.Name == "list_reminders" || tc.Function.Name == "delete_reminder" {
						if injected, err := tools.InjectReminderArgs(args, msg.Platform, msg.UserID, msg.ChatID); err == nil {
							args = injected
						}
					}
					var err error
					result, err = a.tools.ExecuteTool(tc.Function.Name, args)
					if err != nil {
						result = "Error: " + err.Error()
					}
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
			var result string
			if toolName == "spawn_subagents" {
				result = a.handleSpawnSubagents(ctx, toolArgs, msg)
			} else {
				if toolName == "save_memory" || toolName == "read_memory" {
					if injected, err := tools.InjectMemoryArgs(toolArgs, msg.Platform, msg.UserID); err == nil {
						toolArgs = injected
					}
				}
				if toolName == "create_scheduled_reminder" || toolName == "list_reminders" || toolName == "delete_reminder" {
					if injected, err := tools.InjectReminderArgs(toolArgs, msg.Platform, msg.UserID, msg.ChatID); err == nil {
						toolArgs = injected
					}
				}
				var err error
				result, err = a.tools.ExecuteTool(toolName, toolArgs)
				if err != nil {
					result = "Error: " + err.Error()
				}
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
		if a.convStore != nil {
			_ = a.convStore.Add(msg.Platform, msg.UserID, "user", text)
			_ = a.convStore.Add(msg.Platform, msg.UserID, "assistant", reply)
		}
		return reply
	}

	return "I hit the tool limit. Please try a simpler request."
}

var (
	rememberPrefix = regexp.MustCompile(`(?i)^(?:remember|memorize)\s*(?:that|:)?\s*`)
	rememberName   = regexp.MustCompile(`(?i)^(?:remember|memorize)\s+my\s+name\s+is\s+(.+)$`)
)

// handleSpawnSubagents parses spawn_subagents args, runs sub-agents concurrently, and returns formatted results.
func (a *Agent) handleSpawnSubagents(ctx context.Context, argsJSON string, msg gateway.IncomingMessage) string {
	var args struct {
		Tasks []string `json:"tasks"`
		Role  string   `json:"role"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "Error: invalid spawn_subagents arguments: " + err.Error()
	}
	if len(args.Tasks) == 0 {
		return "Error: tasks cannot be empty."
	}
	specs := make([]SubtaskSpec, len(args.Tasks))
	for i, t := range args.Tasks {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		specs[i] = SubtaskSpec{Task: t, Role: args.Role, Index: i}
	}
	// Filter out empty tasks
	n := 0
	for _, s := range specs {
		if s.Task != "" {
			specs[n] = s
			n++
		}
	}
	specs = specs[:n]
	if len(specs) == 0 {
		return "Error: no valid tasks provided."
	}
	results, err := a.RunSubagents(ctx, specs, msg, nil)
	if err != nil {
		return "Error running sub-agents: " + err.Error()
	}
	return FormatSubagentResults(results)
}

// extractRememberContent returns content to save when user explicitly asks to remember something.
func extractRememberContent(text string) string {
	text = strings.TrimSpace(text)
	if len(text) < 10 {
		return ""
	}
	lower := strings.ToLower(text)
	if !strings.HasPrefix(lower, "remember") && !strings.HasPrefix(lower, "memorize") {
		return ""
	}
	// "remember my name is X" -> "User's name is X"
	if m := rememberName.FindStringSubmatch(text); len(m) > 1 {
		name := strings.TrimSpace(m[1])
		if name != "" {
			return "User's name is " + name
		}
	}
	// "remember that X" or "remember: X" or "remember X"
	if loc := rememberPrefix.FindStringIndex(text); loc != nil {
		content := strings.TrimSpace(text[loc[1]:])
		if len(content) >= 3 {
			return content
		}
	}
	return ""
}
