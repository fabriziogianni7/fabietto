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
	"custom-agent/skills"
	"custom-agent/tools"
	"custom-agent/wallet/redact"

	"github.com/sashabaranov/go-openai"
)

const (
	agentParentModel = "moonshotai/kimi-k2-instruct-0905" // default when parentModel not specified
	maxToolRounds    = 10
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
	client               *openai.Client
	parentModel          string                   // model for chat completion; when empty, use default
	subagentModel        string                   // model for subagents; when empty, use subagentModelForIndex (Groq rotation)
	subagentModelForRole func(role string) string // optional; when set and role non-empty, overrides subagentModel
	systemPrompt         string
	compactor            *compaction.Compactor
	skipCompaction       bool // when true, bypass compaction (e.g. autonomous mode)
	tools                *tools.Tools
	memoryStore          *memory.Store
	convStore            *conversation.Store
	skillsDir            string
	skillsMgr            *skills.Manager
}

// New creates an Agent with the given LLM client, system prompt, tools, and optional stores.
// parentModel: model for chat completion; when empty, use default (Groq-specific).
// subagentModel: model for subagents; when empty, use Groq rotation.
// tokenThreshold: when context exceeds this (approx tokens), compaction is triggered. 0 = default (4000).
// skipCompaction: when true, bypass compaction entirely (e.g. autonomous mode).
// skillsDir: optional path to skills directory; when set, skill descriptions are injected into system prompt.
// modelForRole: optional; when non-nil and spawn_subagents uses role, returns model for that role (autonomous mode).
func New(client *openai.Client, parentModel string, subagentModel string, systemPrompt string, tokenThreshold int, skipCompaction bool, toolSet *tools.Tools, convStore *conversation.Store, skillsDir string, modelForRole func(role string) string) *Agent {
	if parentModel == "" {
		parentModel = agentParentModel
	}
	var skillsMgr *skills.Manager
	if skillsDir != "" {
		skillsMgr = skills.NewManager(skillsDir)
	}
	return &Agent{
		client:               client,
		parentModel:          parentModel,
		subagentModel:        subagentModel,
		subagentModelForRole: modelForRole,
		systemPrompt:         systemPrompt,
		compactor:            compaction.NewCompactor(client, parentModel, tokenThreshold),
		skipCompaction:       skipCompaction,
		tools:                toolSet,
		memoryStore:          toolSet.MemoryStore,
		convStore:            convStore,
		skillsDir:            skillsDir,
		skillsMgr:            skillsMgr,
	}
}

// HandleMessage processes an incoming message and returns the reply.
func (a *Agent) HandleMessage(ctx context.Context, msg gateway.IncomingMessage) string {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return "Hello! Send me a message and I'll respond."
	}

	// Handle newSkill command (interactive flow)
	if handled, res := a.handleNewSkill(ctx, text, msg.Platform, msg.UserID); handled {
		return res
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
				log.Printf("[agent] proactively saved memory: %s", redact.Redact(content))
			}
		}
	}

	// Handle approval messages
	if cmd, ok := tools.ParseApprovalMessage(text); ok {
		if handled, result := a.tools.TryWalletApproval(ctx, cmd, msg.Platform, msg.UserID, msg.ChatID); handled {
			return result
		}
		if err := tools.ApproveCommand(cmd); err != nil {
			return "Approval failed: " + err.Error()
		}
		return "Approved."
	}

	history, err := session.Load(msg.Platform, msg.UserID)
	if err != nil {
		// log handled by caller if needed
	}

	// Threshold-based compaction: summarize old context when it exceeds limit (skipped in autonomous mode)
	var summaryBlock string
	var recent []session.Message
	if a.skipCompaction {
		recent = session.Recent(history)
	} else {
		summaryBlock, recent, _ = a.compactor.CompactIfNeeded(ctx, history, a.systemPrompt)
		if recent == nil {
			recent = session.Recent(history)
		}
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

	systemContent := a.systemPrompt
	if a.skillsMgr != nil {
		if block := a.buildSkillsDescriptionBlock(); block != "" {
			systemContent += "\n\n" + block
		}
	}
	messages := make([]openai.ChatCompletionMessage, 0, len(recent)+5)
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: systemContent,
	})
	// When user wants to send and we have prior context, inject reminder so LLM doesn't repeat prior "Done!" without calling the tool
	if a.tools.Wallet != nil && len(recent) > 0 && wantsWalletSend(text) {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: "CRITICAL: The user is asking to send funds. You MUST call wallet_execute_transfer now. Do NOT respond with text claiming you sent—only a tool call actually executes. Reply with a tool call.",
		})
	}
	for _, block := range contextBlocks {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: block,
		})
	}
	if summaryBlock != "" {
		log.Printf("[agent] context compacted: %d history messages → summary + %d recent", len(history), len(recent))
		log.Printf("[agent] compacted summary:\n%s", redact.Redact(summaryBlock))
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
	mustExecuteWallet := a.tools.Wallet != nil && wantsWalletSend(text)
	walletToolUsed := false

	for i := 0; i < maxToolRounds; i++ {
		sendMessages := messages
		if a.skipCompaction {
			// x402 router with model "auto" may route to Anthropic Messages API, which expects
			// top-level "system" parameter, not system role in messages. Prepend system to first user message.
			sendMessages = convertToMessagesAPIFormat(messages)
		}
		req := openai.ChatCompletionRequest{
			Model:    a.parentModel,
			Messages: sendMessages,
			Tools:    toolDefs,
		}
		if a.skipCompaction {
			req.MaxTokens = 4096 // x402 router requires max_tokens
		}
		resp, err := a.client.CreateChatCompletion(ctx, req)
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
				if tc.Function.Name == "wallet_execute_transfer" || tc.Function.Name == "wallet_execute_contract_call" {
					walletToolUsed = true
				}
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
					if tc.Function.Name == "wallet_execute_transfer" || tc.Function.Name == "wallet_execute_contract_call" {
						if injected, err := tools.InjectWalletArgs(args, msg.Platform, msg.UserID, msg.ChatID); err == nil {
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
			if toolName == "wallet_execute_transfer" || toolName == "wallet_execute_contract_call" {
				walletToolUsed = true
			}
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
				if toolName == "wallet_execute_transfer" || toolName == "wallet_execute_contract_call" {
					if injected, err := tools.InjectWalletArgs(toolArgs, msg.Platform, msg.UserID, msg.ChatID); err == nil {
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
		if mustExecuteWallet && !walletToolUsed && claimsWalletWasSent(reply) {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleSystem,
				Content: "You claimed a wallet transaction was sent, but no wallet execution tool was called in this turn. Do not claim success. Call wallet_execute_transfer or wallet_execute_contract_call, or ask a clarifying question if details are missing.",
			})
			continue
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
	rememberPrefix   = regexp.MustCompile(`(?i)^(?:remember|memorize)\s*(?:that|:)?\s*`)
	rememberName     = regexp.MustCompile(`(?i)^(?:remember|memorize)\s+my\s+name\s+is\s+(.+)$`)
	walletSendIntent = regexp.MustCompile(`(?i)(send|transfer|pay|invio)\s+.*(0x[0-9a-fA-F]{40}|eth|wei|matic)|0x[0-9a-fA-F]{40}.*(send|transfer)`)
	walletSentClaim  = regexp.MustCompile(`(?i)(done!? i've sent|i've sent|i sent|transaction hash|hash:\s*0x|explorer:\s*https?://|sent\s+.*\s+to\s+0x[0-9a-fA-F]{40})`)
)

func wantsWalletSend(text string) bool {
	text = strings.TrimSpace(text)
	if len(text) < 8 {
		return false
	}
	return walletSendIntent.MatchString(text)
}

func claimsWalletWasSent(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	return walletSentClaim.MatchString(text)
}

const maxSkillDescriptionsLen = 1500

func (a *Agent) buildSkillsDescriptionBlock() string {
	if a.skillsMgr == nil {
		return ""
	}
	list, err := a.skillsMgr.List()
	if err != nil || len(list) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("--- Available skills (use list_skills / read_skill to inspect) ---\n")
	n := 0
	for _, s := range list {
		line := "- " + s.Name + ": " + s.Description + "\n"
		if b.Len()+len(line) > maxSkillDescriptionsLen {
			break
		}
		b.WriteString(line)
		n++
	}
	b.WriteString("--- End skills ---")
	return b.String()
}

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

// convertToMessagesAPIFormat removes system-role messages and prepends their content to the first
// user message. The x402 router with model "auto" may route to Anthropic's Messages API, which
// rejects "system" as a message role and expects system as a top-level parameter. Prepending
// to the first user message is a compatible workaround.
func convertToMessagesAPIFormat(messages []openai.ChatCompletionMessage) []openai.ChatCompletionMessage {
	var systemParts []string
	var out []openai.ChatCompletionMessage
	for _, m := range messages {
		if m.Role == openai.ChatMessageRoleSystem {
			if m.Content != "" {
				systemParts = append(systemParts, m.Content)
			}
			continue
		}
		out = append(out, m)
	}
	if len(systemParts) == 0 || len(out) == 0 {
		return out
	}
	// Prepend system to first user message
	systemBlock := strings.Join(systemParts, "\n\n")
	for i := range out {
		if out[i].Role == openai.ChatMessageRoleUser {
			out[i].Content = systemBlock + "\n\n---\n\n" + out[i].Content
			break
		}
	}
	return out
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
