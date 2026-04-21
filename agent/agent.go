package agent

import (
	"context"
	"encoding/json"
	"log"
	"regexp"
	"strings"
	"time"

	"custom-agent/agent/planning"
	"custom-agent/compaction"
	"custom-agent/conversation"
	"custom-agent/gateway"
	"custom-agent/memory"
	"custom-agent/session"
	"custom-agent/skills"
	"custom-agent/telemetry"
	"custom-agent/tools"
	"custom-agent/wallet/redact"

	"github.com/sashabaranov/go-openai"
)

const (
	parentModel   = "openai/gpt-oss-120b"
	maxToolRounds = 1000
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
	client        *openai.Client
	systemPrompt  string
	compactor     *compaction.Compactor
	tools         *tools.Tools
	memoryStore   *memory.Store
	convStore     *conversation.Store
	skillsDir     string
	skillsMgr     *skills.Manager
	telemetry     *telemetry.Runtime
	planner       planning.RuntimeConfig
	orchestration planning.OrchestrationRuntime
}

// New creates an Agent with the given LLM client, system prompt, tools, and optional stores.
// tokenThreshold: when context exceeds this (approx tokens), compaction is triggered. 0 = default (4000).
// skillsDir: optional path to skills directory; when set, skill descriptions are injected into system prompt.
func New(client *openai.Client, systemPrompt string, tokenThreshold int, toolSet *tools.Tools, convStore *conversation.Store, skillsDir string, telem *telemetry.Runtime) *Agent {
	var skillsMgr *skills.Manager
	if skillsDir != "" {
		skillsMgr = skills.NewManager(skillsDir)
	}
	return &Agent{
		client:        client,
		systemPrompt:  systemPrompt,
		compactor:     compaction.NewCompactor(client, parentModel, tokenThreshold),
		tools:         toolSet,
		memoryStore:   toolSet.MemoryStore,
		convStore:     convStore,
		skillsDir:     skillsDir,
		skillsMgr:     skillsMgr,
		telemetry:     telem,
		planner:       planning.RuntimeConfig{Enabled: false},
		orchestration: planning.OrchestrationRuntime{Mode: planning.OrchestrationModeOff},
	}
}

// SetPlanner configures plan-and-execute (e.g. from PLANNER_ENABLED / PLANNER_CAPABILITIES).
func (a *Agent) SetPlanner(cfg planning.RuntimeConfig) {
	if a != nil {
		a.planner = cfg
		if strings.TrimSpace(cfg.Orchestration.Mode) != "" {
			a.orchestration = cfg.Orchestration
		} else {
			a.orchestration = planning.OrchestrationRuntime{Mode: planning.OrchestrationModeOff}
		}
	}
}

// HandleMessage processes an incoming message and returns the reply.
func (a *Agent) HandleMessage(ctx context.Context, msg gateway.IncomingMessage) string {
	text := strings.TrimSpace(msg.Text)
	finalReply := ""
	if text == "" {
		finalReply = "Hello! Send me a message and I'll respond."
		return finalReply
	}
	sessionID := session.SessionKey(msg.Platform, msg.UserID)
	var turn *telemetry.TurnRecorder
	if a.telemetry != nil {
		turn = a.telemetry.StartTurn(msg.Platform, sessionID, parentModel, msg.Platform)
		ctx = a.telemetry.WithTurn(ctx, turn)
		turn.AddTranscript(telemetry.TranscriptEntry{Role: "user", Content: text, Kind: "user_message"})
		defer func() {
			if finalReply == "" {
				finalReply = "(no final response)"
			}
			a.telemetry.FinishTurn(turn, finalReply)
		}()
	}

	// Handle newSkill command (interactive flow)
	if handled, res := a.handleNewSkill(ctx, text, msg.Platform, msg.UserID); handled {
		finalReply = res
		return finalReply
	}

	// Handle /new command
	if text == "/new" {
		if err := session.Clear(msg.Platform, msg.UserID); err != nil {
			finalReply = "Failed to clear session: " + err.Error()
			return finalReply
		}
		_ = conversation.Clear(msg.Platform, msg.UserID)
		finalReply = "Session cleared. Starting fresh!"
		return finalReply
	}

	// /remember and NL "save this chat" — promote session tail to long-term memory (before single-fact remember)
	if reply, handled := a.trySessionRemember(text, msg.Platform, msg.UserID); handled {
		finalReply = reply
		return finalReply
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
			finalReply = result
			return finalReply
		}
		if err := tools.ApproveCommand(cmd); err != nil {
			finalReply = "Approval failed: " + err.Error()
			return finalReply
		}
		finalReply = "Approved."
		return finalReply
	}

	history, err := session.Load(msg.Platform, msg.UserID)
	if err != nil {
		// log handled by caller if needed
	}

	// Threshold-based compaction: summarize old context when it exceeds limit
	totalTokens := compaction.EstimateTokens(history)
	if a.telemetry != nil {
		a.telemetry.RecordCompactionDecision(ctx, totalTokens > a.compactor.Threshold(), totalTokens, a.compactor.Threshold())
	}
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
			if a.telemetry != nil {
				a.telemetry.RecordMemoryDecision(ctx, "memory", text, len(mems), a.memoryStore.SearchMode())
			}
		} else if a.telemetry != nil {
			a.telemetry.RecordMemoryDecision(ctx, "memory", text, 0, a.memoryStore.SearchMode())
		}
	}
	if a.convStore != nil {
		if entries, err := a.convStore.Search(msg.Platform, msg.UserID, text, 5); err == nil && len(entries) > 0 {
			contextBlocks = append(contextBlocks, conversation.FormatEntries(entries))
			if a.telemetry != nil {
				a.telemetry.RecordMemoryDecision(ctx, "conversation", text, len(entries), "embedding_or_keyword")
			}
		} else if a.telemetry != nil {
			a.telemetry.RecordMemoryDecision(ctx, "conversation", text, 0, "embedding_or_keyword")
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
			Content: "CRITICAL: The user is asking to send funds. For native coin use wallet_execute_transfer; for ERC-20 tokens (USDC, etc.) use wallet_execute_erc20_transfer. Do NOT respond with text claiming you sent—only a tool call actually executes. Reply with a tool call.",
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

	// Same context as the reactive loop minus the current user line: identity/skills, wallet nudge,
	// memory & conversation blocks, compaction summary, recent session turns.
	orchestrationContextPrefix := messages[:len(messages)-1]

	if a.shouldTryGeneralOrchestration(ctx, text, orchestrationContextPrefix) {
		if reply, ok := a.tryGeneralOrchestration(ctx, msg, text, orchestrationContextPrefix); ok {
			finalReply = reply
			return finalReply
		}
	}

	if a.orchestration.Mode == planning.OrchestrationModeOff {
		if a.shouldUseWalletPlanner(ctx, text) {
			if reply, ok := a.tryWalletPlanner(ctx, msg, text); ok {
				finalReply = reply
				return finalReply
			}
		}
	}

	toolDefs := tools.Definitions()
	mustExecuteWallet := a.tools.Wallet != nil && wantsWalletSend(text)
	walletToolUsed := false
	walletPipelineReactive := a.reactiveWalletContractPipeline()

	for i := 0; i < maxToolRounds; i++ {
		req := openai.ChatCompletionRequest{
			Model:    parentModel,
			Messages: messages,
			Tools:    toolDefs,
		}
		t0 := time.Now()
		resp, err := a.client.CreateChatCompletion(ctx, req)
		a.recordLLMRound(ctx, "reactive_loop", i, req, resp, err, t0)
		if err != nil {
			log.Printf("[agent] LLM error: %v", err)
			finalReply = "Sorry, I couldn't process that. Please try again."
			return finalReply
		}

		if len(resp.Choices) == 0 {
			finalReply = "I didn't get a response. Try again?"
			return finalReply
		}

		msgResp := resp.Choices[0].Message

		// Execute tool calls (structured)
		if len(msgResp.ToolCalls) > 0 {
			messages = append(messages, msgResp)
			for _, tc := range msgResp.ToolCalls {
				if tc.Function.Name == "wallet_execute_transfer" || tc.Function.Name == "wallet_execute_erc20_transfer" || tc.Function.Name == "wallet_execute_erc20_approve" || tc.Function.Name == "wallet_execute_contract_call" {
					walletToolUsed = true
				}
				args := tc.Function.Arguments
				var result string
				result = a.executeToolForMessage(ctx, msg, tc.Function.Name, args, walletPipelineReactive)
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
			if a.telemetry != nil {
				a.telemetry.RecordFallbackParser(ctx, true)
			}
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: msgResp.Content,
			})
			if toolName == "wallet_execute_transfer" || toolName == "wallet_execute_erc20_transfer" || toolName == "wallet_execute_erc20_approve" || toolName == "wallet_execute_contract_call" {
				walletToolUsed = true
			}
			var result string
			result = a.executeToolForMessage(ctx, msg, toolName, toolArgs, walletPipelineReactive)
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
			finalReply = "I didn't get a response. Try again?"
			return finalReply
		}
		if mustExecuteWallet && !walletToolUsed && claimsWalletWasSent(reply) {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleSystem,
				Content: "You claimed a wallet transaction was sent, but no wallet execution tool was called in this turn. Do not claim success. Call wallet_execute_transfer (native), wallet_execute_erc20_transfer (ERC-20 send), wallet_execute_erc20_approve (ERC-20 allowance), or wallet_execute_contract_call, or ask a clarifying question if details are missing.",
			})
			continue
		}
		_ = session.Append(msg.Platform, msg.UserID, text, reply)
		if a.convStore != nil {
			_ = a.convStore.Add(msg.Platform, msg.UserID, "user", text)
			_ = a.convStore.Add(msg.Platform, msg.UserID, "assistant", reply)
		}
		finalReply = reply
		return finalReply
	}

	finalReply = "I hit the tool limit. Please try a simpler request."
	return finalReply
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
