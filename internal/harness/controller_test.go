package harness_test

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"testing"

	"github.com/neoFinch/agent-harness/internal/harness"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

type fakeModel struct {
	respond func(context.Context, *model.LLMRequest, bool) iter.Seq2[*model.LLMResponse, error]
}

func (*fakeModel) Name() string { return "offline-test" }
func (m *fakeModel) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return m.respond(ctx, req, stream)
}
func reply(value string, partial bool) *model.LLMResponse {
	return &model.LLMResponse{Content: genai.NewContentFromText(value, genai.RoleModel), Partial: partial, TurnComplete: !partial}
}

func TestStreamedConversationIsDurable(t *testing.T) {
	dir := t.TempDir()
	llm := &fakeModel{respond: func(_ context.Context, _ *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
		return func(yield func(*model.LLMResponse, error) bool) {
			if !stream {
				t.Fatal("streaming not requested")
			}
			if !yield(reply("Hello ", true), nil) {
				return
			}
			if !yield(reply("ADK", true), nil) {
				return
			}
			yield(reply("Hello ADK", false), nil)
		}
	}}
	c, err := harness.Open(t.Context(), harness.Config{DataDir: dir, Model: llm})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	id := c.SessionID()
	var visible strings.Builder
	err = c.Run(t.Context(), "hello", func(chunk string) {
		visible.WriteString(chunk)
		turns, err := c.Transcript(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if len(turns) != 1 || turns[0].Text != visible.String() {
			t.Fatalf("display preceded persistence: %+v", turns)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if visible.String() != "Hello ADK" {
		t.Fatalf("duplicated output: %q", visible.String())
	}
	turns, err := c.Transcript(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].ID == "" || turns[0].InvocationID == "" || turns[0].Prompt != "hello" || turns[0].Status != "completed" || turns[0].Text != "Hello ADK" {
		t.Fatalf("saved turn=%+v", turns)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := harness.Open(t.Context(), harness.Config{DataDir: dir, Model: llm})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	recovered, err := reopened.Transcript(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 || recovered[0].ID != turns[0].ID || recovered[0].Text != "Hello ADK" {
		t.Fatalf("disk reload=%+v", recovered)
	}
}

func requestBody(req *model.LLMRequest) string {
	var b strings.Builder
	for _, c := range req.Contents {
		for _, p := range c.Parts {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

func TestFailedTurnIsSavedButExcludedFromNextModelHistory(t *testing.T) {
	attempts := 0
	llm := &fakeModel{respond: func(_ context.Context, req *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
		return func(yield func(*model.LLMResponse, error) bool) {
			attempts++
			if attempts == 1 {
				if !yield(reply("unfinished", true), nil) {
					return
				}
				yield(nil, errors.New("connection lost"))
				return
			}
			if strings.Contains(requestBody(req), "failed prompt") || strings.Contains(requestBody(req), "unfinished") {
				t.Fatalf("failed exchange replayed: %s", requestBody(req))
			}
			if !strings.Contains(requestBody(req), "next prompt") {
				t.Fatal("current prompt missing")
			}
			yield(reply("completed answer", false), nil)
		}
	}}
	c, err := harness.Open(t.Context(), harness.Config{DataDir: t.TempDir(), Model: llm})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Run(t.Context(), "failed prompt", nil); err == nil {
		t.Fatal("lost stream failure")
	}
	if err := c.Run(t.Context(), "next prompt", nil); err != nil {
		t.Fatal(err)
	}
	turns, err := c.Transcript(t.Context(), c.SessionID())
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[0].Prompt != "failed prompt" || turns[0].Text != "unfinished" || turns[0].Status != "failed" || turns[1].Status != "completed" {
		t.Fatalf("turns=%+v", turns)
	}
}

func TestConversationContinuesAndClearOnlyResetsActiveHistory(t *testing.T) {
	step := 0
	llm := &fakeModel{respond: func(_ context.Context, req *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
		return func(yield func(*model.LLMResponse, error) bool) {
			step++
			body := requestBody(req)
			if step == 2 && (!strings.Contains(body, "remember mango") || !strings.Contains(body, "answer 1")) {
				t.Fatalf("history lost: %s", body)
			}
			if step == 3 && (strings.Contains(body, "remember mango") || strings.Contains(body, "answer 1") || strings.Contains(body, "follow up")) {
				t.Fatalf("clear did not reset: %s", body)
			}
			yield(reply(fmt.Sprintf("answer %d", step), false), nil)
		}
	}}
	c, err := harness.Open(t.Context(), harness.Config{DataDir: t.TempDir(), Model: llm})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	id := c.SessionID()
	for _, prompt := range []string{"remember mango", "follow up"} {
		if err := c.Run(t.Context(), prompt, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Clear(t.Context()); err != nil {
		t.Fatal(err)
	}
	if c.SessionID() != id {
		t.Fatal("clear changed session identity without product approval")
	}
	if err := c.Run(t.Context(), "fresh prompt", nil); err != nil {
		t.Fatal(err)
	}
	turns, err := c.Transcript(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 3 || turns[0].Prompt != "remember mango" {
		t.Fatalf("clear deleted transcript: %+v", turns)
	}
}

func TestCancelledTurnAndStorageFailures(t *testing.T) {
	t.Run("cancelled", func(t *testing.T) {
		llm := &fakeModel{respond: func(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
			return func(yield func(*model.LLMResponse, error) bool) {
				if !yield(reply("saved first", true), nil) {
					return
				}
				yield(reply("not displayed", true), nil)
			}
		}}
		c, err := harness.Open(t.Context(), harness.Config{DataDir: t.TempDir(), Model: llm})
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		err = c.Run(ctx, "hello", func(string) { cancel() })
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v", err)
		}
		turns, err := c.Transcript(t.Context(), c.SessionID())
		if err != nil {
			t.Fatal(err)
		}
		if len(turns) != 1 || turns[0].Status != "cancelled" || turns[0].Text != "saved first" {
			t.Fatalf("turns=%+v", turns)
		}
	})
	t.Run("unwritable storage blocks generation", func(t *testing.T) {
		called := false
		llm := &fakeModel{respond: func(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
			called = true
			return nil
		}}
		c, err := harness.Open(t.Context(), harness.Config{DataDir: t.TempDir(), Model: llm})
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
		if err := c.Run(t.Context(), "hello", nil); err == nil {
			t.Fatal("storage failure hidden")
		}
		if called {
			t.Fatal("model called before input was saved")
		}
	})
	t.Run("mid stream storage failure", func(t *testing.T) {
		dir := t.TempDir()
		llm := &fakeModel{respond: func(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
			return func(yield func(*model.LLMResponse, error) bool) {
				if !yield(reply("saved", true), nil) {
					return
				}
				yield(reply("unsaved", true), nil)
			}
		}}
		c, err := harness.Open(t.Context(), harness.Config{DataDir: dir, Model: llm})
		if err != nil {
			t.Fatal(err)
		}
		id := c.SessionID()
		var visible strings.Builder
		err = c.Run(t.Context(), "hello", func(chunk string) {
			visible.WriteString(chunk)
			if err := c.Close(); err != nil {
				t.Fatal(err)
			}
		})
		if err == nil || visible.String() != "saved" {
			t.Fatalf("err=%v output=%q", err, visible.String())
		}
		c, err = harness.Open(t.Context(), harness.Config{DataDir: dir, Model: llm})
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		turns, err := c.Transcript(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if len(turns) != 1 || turns[0].Status != "incomplete" || turns[0].Text != "saved" {
			t.Fatalf("turns=%+v", turns)
		}
	})
}

func TestTruncatedReplyIsNotTreatedAsCompletedHistory(t *testing.T) {
	step := 0
	llm := &fakeModel{respond: func(_ context.Context, req *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
		return func(yield func(*model.LLMResponse, error) bool) {
			step++
			if step == 1 {
				r := reply("cut off", false)
				r.FinishReason = genai.FinishReasonMaxTokens
				yield(r, nil)
				return
			}
			if strings.Contains(requestBody(req), "cut off") || strings.Contains(requestBody(req), "too long") {
				t.Fatalf("truncated exchange replayed: %s", requestBody(req))
			}
			yield(reply("new answer", false), nil)
		}
	}}
	c, err := harness.Open(t.Context(), harness.Config{DataDir: t.TempDir(), Model: llm})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Run(t.Context(), "too long", nil); err == nil {
		t.Fatal("truncated generation treated as success")
	}
	if err := c.Run(t.Context(), "try again", nil); err != nil {
		t.Fatal(err)
	}
	turns, err := c.Transcript(t.Context(), c.SessionID())
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[0].Status != "failed" || turns[0].Text != "cut off" || turns[1].Status != "completed" {
		t.Fatalf("turns=%+v", turns)
	}
}
