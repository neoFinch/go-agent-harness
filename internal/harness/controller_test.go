package harness_test

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/neoFinch/agent-harness/internal/harness"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/session/database"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

type fakeModel struct {
	name    string
	respond func(context.Context, *model.LLMRequest, bool) iter.Seq2[*model.LLMResponse, error]
}

func (m *fakeModel) Name() string {
	if m.name != "" {
		return m.name
	}
	return "offline-test"
}
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

func TestResumeRestoresHistoryAndAppendsToOriginalSession(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	llm := &fakeModel{respond: func(_ context.Context, req *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
		return func(yield func(*model.LLMResponse, error) bool) {
			calls++
			if calls == 2 && (!strings.Contains(requestBody(req), "remember 3412") || !strings.Contains(requestBody(req), "saved code")) {
				t.Errorf("resume lost history: %s", requestBody(req))
			}
			yield(reply("saved code", false), nil)
		}
	}}
	c, err := harness.Open(t.Context(), harness.Config{DataDir: dir, Model: llm})
	if err != nil {
		t.Fatal(err)
	}
	id := c.SessionID()
	if err := c.Run(t.Context(), "remember 3412", nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	c, err = harness.Open(t.Context(), harness.Config{DataDir: dir, Model: llm, SessionID: id})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.SessionID() != id {
		t.Fatal("resume changed identity")
	}
	if err := c.Run(t.Context(), "what was the code?", nil); err != nil {
		t.Fatal(err)
	}
	turns, err := c.Transcript(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[1].Prompt != "what was the code?" {
		t.Fatalf("transcript=%+v", turns)
	}
	sessions, err := c.Sessions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].ID != id {
		t.Fatalf("sessions=%+v", sessions)
	}
	if calls != 2 {
		t.Fatalf("resume triggered extra generation: %d", calls)
	}
}

func TestResumeRestoresClearCheckpointAndModelWhileNewUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	respond := func(_ context.Context, req *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
		return func(yield func(*model.LLMResponse, error) bool) { yield(reply("answer", false), nil) }
	}
	alpha := &fakeModel{name: "alpha", respond: respond}
	c, err := harness.Open(t.Context(), harness.Config{DataDir: dir, Model: alpha})
	if err != nil {
		t.Fatal(err)
	}
	id := c.SessionID()
	if err := c.Run(t.Context(), "discarded secret", nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Clear(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := c.Run(t.Context(), "retained fact", nil); err != nil {
		t.Fatal(err)
	}
	c.Close()
	var seen []string
	resumed := &fakeModel{name: "alpha", respond: func(_ context.Context, req *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
		return func(yield func(*model.LLMResponse, error) bool) {
			body := requestBody(req)
			if strings.Contains(body, "discarded secret") || !strings.Contains(body, "retained fact") {
				t.Errorf("bad resumed checkpoint: %s", body)
			}
			yield(reply("restored", false), nil)
		}
	}}
	beta := &fakeModel{name: "beta", respond: func(_ context.Context, req *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
		return func(yield func(*model.LLMResponse, error) bool) {
			if strings.Contains(requestBody(req), "retained fact") {
				t.Error("new session inherited history")
			}
			yield(reply("fresh", false), nil)
		}
	}}
	c, err = harness.Open(t.Context(), harness.Config{DataDir: dir, Model: beta, SessionID: id, ModelFactory: func(_ context.Context, name string) (model.LLM, error) {
		seen = append(seen, name)
		return resumed, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.Settings().Model != "alpha" || len(seen) != 1 || seen[0] != "alpha" {
		t.Fatalf("settings=%+v factory=%v", c.Settings(), seen)
	}
	if err := c.Run(t.Context(), "continue", nil); err != nil {
		t.Fatal(err)
	}
	if err := c.NewSession(t.Context()); err != nil {
		t.Fatal(err)
	}
	freshID := c.SessionID()
	if freshID == id || c.Settings().Model != "beta" {
		t.Fatalf("new session=%s settings=%+v", freshID, c.Settings())
	}
	if err := c.Resume(t.Context(), "missing"); err == nil {
		t.Fatal("unknown session accepted")
	}
	if c.SessionID() != freshID {
		t.Fatal("failed resume changed active session")
	}
	if err := c.Run(t.Context(), "fresh request", nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Resume(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if c.Settings().Model != "alpha" {
		t.Fatal("sibling settings overwritten")
	}
	turns, err := c.Transcript(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 3 || turns[0].Prompt != "discarded secret" {
		t.Fatalf("new/clear lost original records: %+v", turns)
	}
}

func TestResumeRejectsInvalidCheckpointsWithoutOverwritingSessions(t *testing.T) {
	dir := t.TempDir()
	llm := &fakeModel{respond: func(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
		return func(yield func(*model.LLMResponse, error) bool) { yield(reply("ok", false), nil) }
	}}
	c, err := harness.Open(t.Context(), harness.Config{DataDir: dir, Model: llm})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	active := c.SessionID()
	// Inject damaged state through ADK's public session service, rather than
	// depending on its private database schema or manipulating transcript rows.
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "sessions.sqlite")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	svc, err := database.NewSessionServiceFromDB(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []any{"not JSON", 42, `{"version":99}`, `{"version":1,"settings":{"model":"offline-test"},"cleared_through":99999}`} {
		created, err := svc.Create(t.Context(), &session.CreateRequest{AppName: "agent-harness", UserID: "local", State: map[string]any{harness.StateKey: bad}})
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Resume(t.Context(), created.Session.ID()); err == nil || !strings.Contains(err.Error(), "checkpoint") {
			t.Fatalf("bad state accepted: %v err=%v", bad, err)
		}
		if c.SessionID() != active {
			t.Fatal("failed resume changed session")
		}
		reloaded, err := svc.Get(t.Context(), &session.GetRequest{AppName: "agent-harness", UserID: "local", SessionID: created.Session.ID()})
		if err != nil {
			t.Fatal(err)
		}
		value, err := reloaded.Session.State().Get(harness.StateKey)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(value) != fmt.Sprint(bad) {
			t.Fatalf("corrupt state overwritten: %v", value)
		}
	}
	if _, err := harness.Open(t.Context(), harness.Config{DataDir: dir, Model: llm, SessionID: "unknown"}); err == nil {
		t.Fatal("startup accepted unknown session")
	}
	before, err := c.Sessions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 5 {
		t.Fatalf("failed startup created session: %+v", before)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Resume(t.Context(), active); err == nil {
		t.Fatal("storage error hidden")
	}
	if err := c.NewSession(t.Context()); err == nil {
		t.Fatal("new session storage error hidden")
	}
	if c.SessionID() != active {
		t.Fatal("storage failure changed identity")
	}
}

func TestResumePreservesInterruptedOutputButDoesNotReplayIt(t *testing.T) {
	dir := t.TempDir()
	broken := &fakeModel{respond: func(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
		return func(yield func(*model.LLMResponse, error) bool) {
			if yield(reply("partial secret", true), nil) {
				yield(nil, errors.New("disconnect"))
			}
		}
	}}
	c, err := harness.Open(t.Context(), harness.Config{DataDir: dir, Model: broken})
	if err != nil {
		t.Fatal(err)
	}
	id := c.SessionID()
	if err := c.Run(t.Context(), "failed input", nil); err == nil {
		t.Fatal("expected disconnect")
	}
	c.Close()
	next := &fakeModel{respond: func(_ context.Context, req *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
		return func(yield func(*model.LLMResponse, error) bool) {
			if strings.Contains(requestBody(req), "partial secret") || strings.Contains(requestBody(req), "failed input") {
				t.Errorf("unfinished exchange replayed: %s", requestBody(req))
			}
			yield(reply("new answer", false), nil)
		}
	}}
	c, err = harness.Open(t.Context(), harness.Config{DataDir: dir, Model: next, SessionID: id})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Run(t.Context(), "continue safely", nil); err != nil {
		t.Fatal(err)
	}
	turns, err := c.Transcript(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[0].Status != "failed" || turns[0].Text != "partial secret" || turns[1].Status != "completed" {
		t.Fatalf("transcript=%+v", turns)
	}
}

func TestResumeUpgradesLegacySessionWithoutDiscardingHistory(t *testing.T) {
	dir := t.TempDir()
	llm := &fakeModel{respond: func(_ context.Context, req *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
		return func(yield func(*model.LLMResponse, error) bool) {
			if strings.Contains(requestBody(req), "old cleared input") || !strings.Contains(requestBody(req), "retained legacy fact") {
				t.Errorf("legacy checkpoint lost: %s", requestBody(req))
			}
			yield(reply("ok", false), nil)
		}
	}}
	// Initialize schema, then construct an S01 fixture with the public ADK
	// service and exported capture record type. S01 had no harness state key.
	initial, err := harness.Open(t.Context(), harness.Config{DataDir: dir, Model: llm})
	if err != nil {
		t.Fatal(err)
	}
	initial.Close()
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "sessions.sqlite")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	svc, err := database.NewSessionServiceFromDB(db)
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.Create(t.Context(), &session.CreateRequest{AppName: "agent-harness", UserID: "local"})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Session.ID()
	for i, prompt := range []string{"old cleared input", "retained legacy fact"} {
		invocation := fmt.Sprintf("legacy-%d", i)
		for _, content := range []*genai.Content{genai.NewContentFromText(prompt, genai.RoleUser), genai.NewContentFromText("legacy answer", genai.RoleModel)} {
			e := session.NewEvent(t.Context(), invocation)
			e.Author = "harness"
			e.Content = content
			if err := svc.AppendEvent(t.Context(), created.Session, e); err != nil {
				t.Fatal(err)
			}
		}
		if err := db.Create(&harness.Turn{ID: invocation, InvocationID: invocation, SessionID: id, Kind: "turn", Prompt: prompt, Text: "legacy answer", Status: "completed"}).Error; err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			if err := db.Create(&harness.Turn{ID: "legacy-clear", SessionID: id, Kind: "clear", Status: "completed"}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	c, err := harness.Open(t.Context(), harness.Config{DataDir: dir, Model: llm, SessionID: id})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Run(t.Context(), "continue", nil); err != nil {
		t.Fatal(err)
	}
	turns, err := c.Transcript(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 3 || turns[0].Prompt != "old cleared input" {
		t.Fatalf("legacy transcript lost: %+v", turns)
	}
}
