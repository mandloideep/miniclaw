// Package summary runs new emails through Ollama to produce a one-line
// summary and a needs-attention flag.
//
// The pipeline:
//  1. Pull unsummarized emails for the account (LEFT JOIN summaries IS NULL).
//  2. For each, ask Ollama for {summary, needs_attention, reason} in JSON.
//  3. Upsert into the summaries table.
//
// Designed to be called by the scheduler right after ingest. Failure on
// one email doesn't abort the batch — bad JSON or model error is logged
// and we move on.
package summary

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/mandloideep/miniclaw/internal/db/sqlcgen"
	"github.com/mandloideep/miniclaw/internal/services/ollama"
)

// Generator is the minimal surface Summarizer needs from Ollama.
type Generator interface {
	Generate(ctx context.Context, req ollama.GenerateRequest) (string, error)
}

// errorReporter is satisfied by *ollama.Service. Optional — used to stash
// the last LLM error so the status banner can show it.
type errorReporter interface {
	SetLastError(string)
}

// Summarizer runs the per-account summarization pass.
type Summarizer struct {
	q     *sqlcgen.Queries
	llm   Generator
	batch int32 // max emails per Summarize call
}

// New wires the summariser against the shared pool and an Ollama client.
// Batch size is small on purpose: local models can take 5-30s per email
// on a cold load, so a single scheduler tick stays responsive instead of
// blocking for minutes the first time a user opens the app.
func New(pool *sql.DB, llm Generator) *Summarizer {
	return &Summarizer{
		q:     sqlcgen.New(pool),
		llm:   llm,
		batch: 8,
	}
}

// Summarize runs one pass for accountID. Returns count of summaries written.
func (s *Summarizer) Summarize(ctx context.Context, accountID int64) (int, error) {
	rows, err := s.q.ListUnsummarizedByAccount(ctx, sqlcgen.ListUnsummarizedByAccountParams{
		AccountID: accountID,
		Limit:     int64(s.batch),
	})
	if err != nil {
		return 0, fmt.Errorf("list unsummarized: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	modelRow, err := s.q.GetAccountModel(ctx, accountID)
	if err != nil {
		return 0, fmt.Errorf("get account model: %w", err)
	}
	model := modelRow.AccountModel
	if model == "" {
		model = modelRow.DefaultModel
	}
	if model == "" {
		return 0, fmt.Errorf("no Ollama model configured for account %d", accountID)
	}

	written := 0
	var (
		consecutiveFail int
		firstErr        error
		failCount       int
	)
	for _, r := range rows {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		out, err := s.summarizeOne(ctx, model, r)
		if err != nil {
			if firstErr == nil {
				firstErr = err
				log.Printf("summary: email %d: %v", r.ID, err)
			}
			failCount++
			consecutiveFail++
			// Bail when the model is clearly not producing usable output
			// (warming, wrong model, refused, etc.) — saves quota and
			// stops log spam without retry storms.
			if consecutiveFail >= 3 {
				log.Printf("summary: aborting batch after %d consecutive failures (model=%s)", consecutiveFail, model)
				break
			}
			continue
		}
		consecutiveFail = 0
		needs := int64(0)
		if out.NeedsAttention {
			needs = 1
		}
		if perr := s.q.UpsertSummary(ctx, sqlcgen.UpsertSummaryParams{
			EmailID:         r.ID,
			Summary:         out.Summary,
			NeedsAttention:  needs,
			AttentionReason: out.Reason,
			Model:           model,
		}); perr != nil {
			log.Printf("summary: persist email %d: %v", r.ID, perr)
			continue
		}
		written++
	}
	if failCount > 1 {
		log.Printf("summary: %d total failures in this batch (first surfaced above)", failCount)
	}
	if reporter, ok := s.llm.(errorReporter); ok {
		if firstErr != nil {
			reporter.SetLastError(fmt.Sprintf("%s: %v", model, firstErr))
		} else if written > 0 {
			reporter.SetLastError("")
		}
	}
	return written, nil
}

type llmOutput struct {
	Summary        string `json:"summary"`
	NeedsAttention bool   `json:"needs_attention"`
	Reason         string `json:"reason"`
}

// summarizeOne builds the structured prompt, asks Ollama for JSON, and
// decodes the reply. Falls back to a no-format-json retry once if the
// model refused under JSON mode — small instruct models sometimes return
// empty responses when forced to JSON.
func (s *Summarizer) summarizeOne(ctx context.Context, model string, r sqlcgen.ListUnsummarizedByAccountRow) (llmOutput, error) {
	body := truncate(r.BodyPlain, 4000) // keep small models happy
	prompt := buildPrompt(r.FromName, r.FromAddress, r.Subject, r.ReceivedAt, body)
	resp, err := s.llm.Generate(ctx, ollama.GenerateRequest{
		Model:       model,
		System:      systemPrompt,
		Prompt:      prompt,
		Temperature: 0.2,
		JSONMode:    true,
	})
	if err != nil && strings.Contains(err.Error(), "empty response") {
		// Retry without JSON mode. We extract the first {...} below.
		resp, err = s.llm.Generate(ctx, ollama.GenerateRequest{
			Model:       model,
			System:      systemPrompt,
			Prompt:      prompt,
			Temperature: 0.2,
		})
	}
	if err != nil {
		return llmOutput{}, err
	}
	jsonChunk := extractJSON(resp)
	var out llmOutput
	if err := json.Unmarshal([]byte(jsonChunk), &out); err != nil {
		// Truncate raw payload so logs stay readable.
		truncated := resp
		if len(truncated) > 200 {
			truncated = truncated[:200] + "…"
		}
		return llmOutput{}, fmt.Errorf("decode llm json: %w (raw=%q)", err, truncated)
	}
	if out.Summary == "" {
		return llmOutput{}, fmt.Errorf("empty summary from llm")
	}
	return out, nil
}

// extractJSON pulls the first balanced {...} block out of a model reply.
// Small instruct models routinely wrap the JSON in prose like
// "Sure, here's the summary: {...}". Returns the original string if no
// brace pair is found.
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return s
	}
	return s[start : end+1]
}

const systemPrompt = `You are an email triage assistant. For every email,
return a single JSON object with exactly three keys:
- "summary": one or two sentences describing what this email is and why
  it was sent. Plain English, no marketing words.
- "needs_attention": true ONLY if the recipient must do something specific
  (reply, decide, attend, pay, RSVP, click a deadline-bound link).
  Newsletters, receipts, marketing, social notifications, and FYI updates
  are false.
- "reason": one short phrase explaining the needs_attention decision.
Output ONLY the JSON object. No prose, no code fences.`

func buildPrompt(fromName, fromAddr, subject, receivedAt, body string) string {
	var b strings.Builder
	b.WriteString("From: ")
	if fromName != "" {
		b.WriteString(fromName)
		b.WriteString(" <")
		b.WriteString(fromAddr)
		b.WriteString(">")
	} else {
		b.WriteString(fromAddr)
	}
	fmt.Fprintf(&b, "\nDate: %s\nSubject: %s\n\n", receivedAt, subject)
	b.WriteString(body)
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n\n[...truncated...]"
}
