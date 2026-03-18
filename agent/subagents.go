package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"custom-agent/gateway"
	"custom-agent/session"
	"custom-agent/tools"

	"github.com/sashabaranov/go-openai"
	"golang.org/x/sync/errgroup"
)

const (
	defaultMaxConcurrency   = 4
	defaultPerChildTimeout  = 20 * time.Second
	defaultMaxChildCount    = 10
	subagentMaxToolRounds   = 5
	subagentRoleInstruction = "You are a focused sub-agent. Answer only the given task. Use read_file, web_search, and read_memory as needed. Do not save memory or schedule reminders."
)

// SubtaskSpec describes a single stateless child task.
type SubtaskSpec struct {
	Task  string // the prompt for the child
	Role  string // optional role override, e.g. "research specialist"
	Index int    // for ordering results
}

// SubtaskResult holds the output of a stateless child.
type SubtaskResult struct {
	Task       string
	Output     string
	Err        error
	Index      int
	DurationMs int64
}

// SubagentOpts configures RunSubagents behavior.
type SubagentOpts struct {
	MaxConcurrency  int           // max concurrent children (default 4)
	PerChildTimeout time.Duration // timeout per child (default 20s)
	MaxChildCount   int           // max children per turn (default 10)
}

func (o *SubagentOpts) applyDefaults() {
	if o.MaxConcurrency <= 0 {
		o.MaxConcurrency = defaultMaxConcurrency
	}
	if o.PerChildTimeout <= 0 {
		o.PerChildTimeout = defaultPerChildTimeout
	}
	if o.MaxChildCount <= 0 {
		o.MaxChildCount = defaultMaxChildCount
	}
}

// RunSubagents runs the given specs concurrently with bounded concurrency.
// Returns results in the same order as specs. Children are stateless and do not write session/memory/reminders.
func (a *Agent) RunSubagents(ctx context.Context, specs []SubtaskSpec, msg gateway.IncomingMessage, opts *SubagentOpts) ([]SubtaskResult, error) {
	if opts == nil {
		opts = &SubagentOpts{}
	}
	opts.applyDefaults()

	if len(specs) > opts.MaxChildCount {
		specs = specs[:opts.MaxChildCount]
	}
	if len(specs) == 0 {
		return nil, nil
	}

	sessionKey := session.SessionKey(msg.Platform, msg.UserID)
	log.Printf("[subagents] session=%s spawning %d children (max_concurrency=%d, timeout=%v)", sessionKey, len(specs), opts.MaxConcurrency, opts.PerChildTimeout)

	start := time.Now()
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(opts.MaxConcurrency)

	results := make([]SubtaskResult, len(specs))
	for i, spec := range specs {
		i, spec := i, spec
		g.Go(func() error {
			start := time.Now()
			r := a.runOneSubagent(gCtx, spec, msg, opts.PerChildTimeout)
			r.DurationMs = time.Since(start).Milliseconds()
			r.Index = spec.Index
			results[i] = r
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		log.Printf("[subagents] session=%s wait error: %v", sessionKey, err)
		return nil, err
	}

	elapsed := time.Since(start)
	errCount := 0
	for _, r := range results {
		if r.Err != nil {
			errCount++
		}
	}
	mergeSize := 0
	for _, r := range results {
		mergeSize += len(r.Output)
	}
	log.Printf("[subagents] session=%s done in %v: %d children, %d errors, merge_size=%d bytes", sessionKey, elapsed, len(results), errCount, mergeSize)
	return results, nil
}

// runOneSubagent executes a single stateless child. It does not write session, memory, or reminders.
func (a *Agent) runOneSubagent(ctx context.Context, spec SubtaskSpec, msg gateway.IncomingMessage, timeout time.Duration) SubtaskResult {
	subCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	systemPrompt := a.systemPrompt + "\n\n" + subagentRoleInstruction
	if spec.Role != "" {
		systemPrompt += "\n\nRole: " + spec.Role
	}

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
		{Role: openai.ChatMessageRoleUser, Content: spec.Task},
	}

	toolDefs := tools.DefinitionsForSubagent()

	for round := 0; round < subagentMaxToolRounds; round++ {
		select {
		case <-subCtx.Done():
			log.Printf("[subagents] child index=%d timeout/cancel: %v", spec.Index, subCtx.Err())
			return SubtaskResult{
				Task:   spec.Task,
				Output: "",
				Err:    subCtx.Err(),
				Index:  spec.Index,
			}
		default:
		}

		model := a.subagentModel
		if model == "" {
			model = subagentModelForIndex(spec.Index)
		}
		sendMessages := messages
		if a.skipCompaction {
			sendMessages = convertToMessagesAPIFormat(messages)
		}
		req := openai.ChatCompletionRequest{
			Model:    model,
			Messages: sendMessages,
			Tools:    toolDefs,
		}
		if a.skipCompaction {
			req.MaxTokens = 4096 // x402 router requires max_tokens
		}
		resp, err := a.client.CreateChatCompletion(subCtx, req)
		if err != nil {
			log.Printf("[subagents] child index=%d LLM error: %v", spec.Index, err)
			return SubtaskResult{
				Task:   spec.Task,
				Output: "",
				Err:    err,
				Index:  spec.Index,
			}
		}

		if len(resp.Choices) == 0 {
			return SubtaskResult{Task: spec.Task, Output: "", Err: nil, Index: spec.Index}
		}

		msgResp := resp.Choices[0].Message

		if len(msgResp.ToolCalls) > 0 {
			messages = append(messages, msgResp)
			for _, tc := range msgResp.ToolCalls {
				args := tc.Function.Arguments
				if tc.Function.Name == "read_memory" {
					if injected, err := tools.InjectMemoryArgs(args, msg.Platform, msg.UserID); err == nil {
						args = injected
					}
				}
				if !tools.IsAllowedForSubagent(tc.Function.Name) {
					messages = append(messages, openai.ChatCompletionMessage{
						Role:       openai.ChatMessageRoleTool,
						Content:    "Error: sub-agents cannot use this tool.",
						ToolCallID: tc.ID,
					})
					continue
				}
				result, err := a.tools.ExecuteTool(tc.Function.Name, args)
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

		if toolName, toolArgs, ok := tools.ParseToolCallFromContent(msgResp.Content); ok {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: msgResp.Content,
			})
			if toolName == "read_memory" {
				if injected, err := tools.InjectMemoryArgs(toolArgs, msg.Platform, msg.UserID); err == nil {
					toolArgs = injected
				}
			}
			if !tools.IsAllowedForSubagent(toolName) {
				messages = append(messages, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    "Error: sub-agents cannot use this tool.",
					ToolCallID: "fallback",
					Name:       toolName,
				})
			} else {
				result, err := a.tools.ExecuteTool(toolName, toolArgs)
				if err != nil {
					result = "Error: " + err.Error()
				}
				messages = append(messages, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    result,
					ToolCallID: "fallback",
					Name:       toolName,
				})
			}
			continue
		}

		reply := strings.TrimSpace(msgResp.Content)
		return SubtaskResult{
			Task:   spec.Task,
			Output: reply,
			Err:    nil,
			Index:  spec.Index,
		}
	}

	return SubtaskResult{
		Task:   spec.Task,
		Output: "",
		Err:    nil,
		Index:  spec.Index,
	}
}

// FormatSubagentResults returns a merged string for the parent to inject into messages.
func FormatSubagentResults(results []SubtaskResult) string {
	if len(results) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("--- Sub-agent results ---\n")
	for i, r := range results {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("Task ")
		b.WriteString(fmt.Sprintf("%d", i+1))
		b.WriteString(": ")
		b.WriteString(": ")
		if r.Err != nil {
			b.WriteString("(error: ")
			b.WriteString(r.Err.Error())
			b.WriteString(")")
		} else {
			b.WriteString(r.Output)
		}
	}
	b.WriteString("\n--- End sub-agent results ---")
	return b.String()
}
