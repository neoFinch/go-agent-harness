package adkprototype

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/genai"
	"path/filepath"
	"strings"
	"testing"
)

func budgetRunner(t *testing.T, svc session.Service, llm model.LLM, gate *BudgetGate, tools ...tool.Tool) *runner.Runner {
	t.Helper()
	a, err := llmagent.New(llmagent.Config{Name: "prototype", Model: llm, Instruction: "Count these instructions too.", BeforeModelCallbacks: []llmagent.BeforeModelCallback{gate.BeforeModel}, Tools: tools})
	if err != nil {
		t.Fatal(err)
	}
	r, err := runner.New(runner.Config{AppName: "prototype", Agent: a, SessionService: svc})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestBudgetDefaultReserveCappedBeforeModelCall(t *testing.T) {
	gate := &BudgetGate{ContextBudget: 1000, ModelWindow: 2000, ModelMaxOutput: 200, Count: func(req *model.LLMRequest) (int, error) { return 100, nil }}
	svc := session.InMemoryService()
	llm := &scriptedModel{t: t, respond: func(_ int, req *model.LLMRequest) []*model.LLMResponse {
		if req.Config == nil || req.Config.MaxOutputTokens != 200 {
			t.Fatalf("expected default 25%% reserve capped at 200, config=%+v", req.Config)
		}
		return []*model.LLMResponse{text("within budget", false)}
	}}
	run(t, t.Context(), budgetRunner(t, svc, llm, gate), createSession(t, svc), "hello")
}

func TestBudgetCompactsAtNinetyPercentBeforeCallingModel(t *testing.T) {
	for _, used := range []int{674, 675, 800} {
		t.Run(fmt.Sprint(used), func(t *testing.T) {
			compactCalls := 0
			gate := &BudgetGate{ContextBudget: 1000, ModelWindow: 2000, ModelMaxOutput: 500,
				Count: func(req *model.LLMRequest) (int, error) {
					if req.Config == nil || req.Config.SystemInstruction == nil {
						t.Fatal("counter did not see instructions")
					}
					if strings.Contains(requestText(req), "compact-summary") {
						return 440, nil
					}
					return used, nil
				},
				Compact: func(_ context.Context, req *model.LLMRequest, targets BudgetTargets) error {
					compactCalls++
					if targets != (BudgetTargets{Output: 250, Input: 750, Target: 450, Recent: 300}) {
						t.Fatalf("targets=%+v", targets)
					}
					// The injected compactor is a deterministic fixture, not a real model.
					req.Contents = []*genai.Content{genai.NewContentFromText("compact-summary", genai.RoleUser), req.Contents[len(req.Contents)-1]}
					return nil
				},
			}
			svc := session.InMemoryService()
			id := createSession(t, svc)
			llm := &scriptedModel{t: t, respond: func(_ int, req *model.LLMRequest) []*model.LLMResponse {
				if req.Config.MaxOutputTokens != 250 {
					t.Fatal("output reserve not enforced")
				}
				if !strings.Contains(requestText(req), "pending prompt") {
					t.Fatal("current prompt lost")
				}
				if used >= 675 && !strings.Contains(requestText(req), "compact-summary") {
					t.Fatal("model received uncompacted input")
				}
				return []*model.LLMResponse{text("done", false)}
			}}
			run(t, t.Context(), budgetRunner(t, svc, llm, gate), id, "pending prompt")
			want := 0
			if used >= 675 {
				want = 1
			}
			if compactCalls != want {
				t.Fatalf("compactions=%d want=%d", compactCalls, want)
			}
			if events(t, svc, id).Events().At(0).Content.Parts[0].Text != "pending prompt" {
				t.Fatal("original transcript changed")
			}
		})
	}
}

func TestBudgetFailedCompactionBlocksModelAndPreservesTranscript(t *testing.T) {
	for _, mode := range []string{"summary_error", "still_oversized"} {
		t.Run(mode, func(t *testing.T) {
			attempts := 0
			gate := &BudgetGate{ContextBudget: 1000, ModelWindow: 2000, ModelMaxOutput: 500,
				Count: func(_ *model.LLMRequest) (int, error) { return 800, nil },
				Compact: func(_ context.Context, req *model.LLMRequest, _ BudgetTargets) error {
					attempts++
					req.Contents = nil // A failed candidate must never replace original history.
					if mode == "summary_error" {
						return errors.New("summary failed")
					}
					return nil
				},
			}
			svc := session.InMemoryService()
			id := createSession(t, svc)
			llm := &scriptedModel{t: t, respond: func(_ int, _ *model.LLMRequest) []*model.LLMResponse {
				t.Fatal("model called after unsuccessful compaction")
				return nil
			}}
			path := filepath.Join(t.TempDir(), "capture.jsonl")
			err := RunCaptured(t.Context(), budgetRunner(t, svc, llm, gate), "user", id, "preserve this pending prompt", path, nil)
			if err == nil || attempts != 3 || llm.step != 0 {
				t.Fatalf("attempts=%d model calls=%d error=%v", attempts, llm.step, err)
			}
			saved := events(t, svc, id)
			if saved.Events().Len() != 1 || saved.Events().At(0).Content.Parts[0].Text != "preserve this pending prompt" {
				t.Fatal("failed candidate changed transcript")
			}
			turns, err := LoadTranscript(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(turns) != 1 || turns[0].Status != "failed" || turns[0].Prompt != "preserve this pending prompt" {
				t.Fatalf("capture=%+v", turns)
			}
		})
	}
}

func TestBudgetChecksToolResultsBeforeFollowupModelCall(t *testing.T) {
	svc := session.InMemoryService()
	id := createSession(t, svc)
	stored := events(t, svc, id)
	for _, entry := range []struct{ role, body string }{{"user", "old question"}, {"model", "old answer"}} {
		e := &session.Event{Author: entry.role, LLMResponse: model.LLMResponse{Content: genai.NewContentFromText(entry.body, genai.Role(entry.role))}}
		if entry.role == "model" {
			e.Author = "prototype"
		}
		if err := svc.AppendEvent(t.Context(), stored, e); err != nil {
			t.Fatal(err)
		}
	}
	checks, compactions := 0, 0
	gate := &BudgetGate{ContextBudget: 1000, ModelWindow: 2000, ModelMaxOutput: 500,
		Count: func(req *model.LLMRequest) (int, error) {
			checks++
			if req.Config == nil || req.Config.SystemInstruction == nil || len(req.Config.Tools) == 0 {
				t.Fatal("counter missed instructions or tool schema")
			}
			result := false
			for _, c := range req.Contents {
				for _, p := range c.Parts {
					if p.FunctionResponse != nil {
						result = true
					}
				}
			}
			if !result {
				return 100, nil
			}
			if strings.Contains(requestText(req), "old question") {
				return 800, nil
			}
			return 440, nil
		},
		Compact: func(_ context.Context, req *model.LLMRequest, targets BudgetTargets) error {
			compactions++
			start := -1
			for i, c := range req.Contents {
				for _, p := range c.Parts {
					if p.Text == "inspect current project" {
						start = i
					}
				}
			}
			if start < 0 {
				return errors.New("pending prompt not found")
			}
			req.Contents = append([]*genai.Content{genai.NewContentFromText("summary of older exchange", genai.RoleUser)}, req.Contents[start:]...)
			return nil
		},
	}
	calls := 0
	read, err := functiontool.New(functiontool.Config{Name: "read_file", Description: "Fixture result that increases input usage"}, func(_ agent.Context, _ readArgs) (readResult, error) {
		calls++
		return readResult{Text: "tool output"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	llm := &scriptedModel{t: t, respond: func(step int, req *model.LLMRequest) []*model.LLMResponse {
		if req.Config.MaxOutputTokens != 250 {
			t.Fatal("output limit missing inside tool loop")
		}
		if step == 1 {
			return []*model.LLMResponse{{Content: &genai.Content{Role: "model", Parts: []*genai.Part{{FunctionCall: &genai.FunctionCall{ID: "call_budget", Name: "read_file", Args: map[string]any{"path": "fixture", "start": 1, "end": 1}}}}}}}
		}
		fc, fr := 0, 0
		for _, c := range req.Contents {
			for _, p := range c.Parts {
				if p.FunctionCall != nil && p.FunctionCall.ID == "call_budget" {
					fc++
				}
				if p.FunctionResponse != nil && p.FunctionResponse.ID == "call_budget" {
					fr++
				}
			}
		}
		if fc != 1 || fr != 1 || strings.Contains(requestText(req), "old question") {
			t.Fatalf("compacted call/result=%d/%d input=%s", fc, fr, requestText(req))
		}
		return []*model.LLMResponse{text("done", false)}
	}}
	run(t, t.Context(), budgetRunner(t, svc, llm, gate, read), id, "inspect current project")
	if llm.step != 2 || calls != 1 || compactions != 1 || checks != 3 {
		t.Fatalf("models=%d tools=%d compactions=%d counts=%d", llm.step, calls, compactions, checks)
	}
	if events(t, svc, id).Events().At(0).Content.Parts[0].Text != "old question" {
		t.Fatal("raw history removed")
	}
}

func TestBudgetPercentageOverrideAndInvalidSettings(t *testing.T) {
	for _, percent := range []int{50, 100, -1} {
		t.Run(fmt.Sprint(percent), func(t *testing.T) {
			gate := &BudgetGate{ContextBudget: 1000, OutputPercent: percent, ModelWindow: 2000, ModelMaxOutput: 900, Count: func(_ *model.LLMRequest) (int, error) { return 100, nil }}
			svc := session.InMemoryService()
			id := createSession(t, svc)
			llm := &scriptedModel{t: t, respond: func(_ int, req *model.LLMRequest) []*model.LLMResponse {
				if percent != 50 {
					t.Fatal("invalid settings reached model")
				}
				if req.Config.MaxOutputTokens != 500 {
					t.Fatal("percentage override not used")
				}
				return []*model.LLMResponse{text("done", false)}
			}}
			err := RunCaptured(t.Context(), budgetRunner(t, svc, llm, gate), "user", id, "hello", filepath.Join(t.TempDir(), "capture.jsonl"), nil)
			if percent == 50 {
				if err != nil || llm.step != 1 {
					t.Fatalf("override error=%v calls=%d", err, llm.step)
				}
			} else if err == nil || llm.step != 0 {
				t.Fatalf("invalid settings error=%v calls=%d", err, llm.step)
			}
		})
	}
}
