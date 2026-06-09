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

// Summarizer runs the per-account summarization pass.
type Summarizer struct {
	q     *sqlcgen.Queries
	llm   Generator
	batch int32 // max emails per Summarize call
}

// New wires the summariser against the shared pool and an Ollama client.
func New(pool *sql.DB, llm Generator) *Summarizer {
	return &Summarizer{
		q:     sqlcgen.New(pool),
		llm:   llm,
		batch: 25,
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
	for _, r := range rows {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		out, err := s.summarizeOne(ctx, model, r)
		if err != nil {
			log.Printf("summary: email %d: %v", r.ID, err)
			continue
		}
		needs := int64(0)
		if out.NeedsAttention {
			needs = 1
		}
		if err := s.q.UpsertSummary(ctx, sqlcgen.UpsertSummaryParams{
			EmailID:         r.ID,
			Summary:         out.Summary,
			NeedsAttention:  needs,
			AttentionReason: out.Reason,
			Model:           model,
		}); err != nil {
			log.Printf("summary: persist email %d: %v", r.ID, err)
			continue
		}
		written++
	}
	return written, nil
}

type llmOutput struct {
	Summary        string `json:"summary"`
	NeedsAttention bool   `json:"needs_attention"`
	Reason         string `json:"reason"`
}

// summarizeOne builds the structured prompt, asks Ollama for JSON, and
// decodes the reply.
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
	if err != nil {
		return llmOutput{}, err
	}
	var out llmOutput
	if err := json.Unmarshal([]byte(resp), &out); err != nil {
		return llmOutput{}, fmt.Errorf("decode llm json: %w (raw=%q)", err, resp)
	}
	if out.Summary == "" {
		return llmOutput{}, fmt.Errorf("empty summary from llm")
	}
	return out, nil
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
