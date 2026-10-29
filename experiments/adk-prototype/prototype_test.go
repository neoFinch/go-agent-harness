// PROTOTYPE: evidence for adopting ADK, not production harness code.
package adkprototype

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/model/openaimodel"
	"google.golang.org/adk/v2/platform"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/session/compaction"
	"google.golang.org/adk/v2/session/database"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/genai"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type scriptedModel struct {
	t       *testing.T
	step    int
	fail    error
	respond func(int, *model.LLMRequest) []*model.LLMResponse
}

func (*scriptedModel) Name() string { return "offline-script" }
func (m *scriptedModel) GenerateContent(_ context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		if !stream {
			m.t.Fatal("runner did not request streaming")
		}
		m.step++
		if m.step > 8 {
			m.t.Fatal("unexpected model loop")
		}
		for _, response := range m.respond(m.step, req) {
			if !yield(response, nil) {
				return
			}
		}
		if m.fail != nil {
			yield(nil, m.fail)
		}
	}
}

func text(s string, partial bool) *model.LLMResponse {
	return &model.LLMResponse{Content: genai.NewContentFromText(s, genai.RoleModel), Partial: partial, TurnComplete: !partial}
}

func requestText(req *model.LLMRequest) string {
	var b strings.Builder
	for _, content := range req.Contents {
		for _, part := range content.Parts {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}

func makeRunner(t *testing.T, svc session.Service, llm model.LLM, tools ...tool.Tool) *runner.Runner {
	t.Helper()
	a, err := llmagent.New(llmagent.Config{Name: "prototype", Model: llm, Instruction: "Use provided tools; do not invent file contents.", Tools: tools})
	if err != nil {
		t.Fatal(err)
	}
	r, err := runner.New(runner.Config{AppName: "prototype", Agent: a, SessionService: svc})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func createSession(t *testing.T, svc session.Service) string {
	t.Helper()
	r, err := svc.Create(t.Context(), &session.CreateRequest{AppName: "prototype", UserID: "user", State: map[string]any{"budget": 32000}})
	if err != nil {
		t.Fatal(err)
	}
	return r.Session.ID()
}

func events(t *testing.T, svc session.Service, id string) session.Session {
	t.Helper()
	r, err := svc.Get(t.Context(), &session.GetRequest{AppName: "prototype", UserID: "user", SessionID: id})
	if err != nil {
		t.Fatal(err)
	}
	return r.Session
}

func run(t *testing.T, ctx context.Context, r *runner.Runner, id, prompt string) []*session.Event {
	t.Helper()
	var result []*session.Event
	for event, err := range r.Run(ctx, "user", id, genai.NewContentFromText(prompt, genai.RoleUser), agent.RunConfig{StreamingMode: agent.StreamingModeSSE}) {
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, event)
	}
	return result
}

func databaseService(t *testing.T, path string) (session.Service, func()) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := database.NewSessionServiceFromDB(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(svc); err != nil {
		t.Fatal(err)
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	return svc, func() {
		if err := connection.Close(); err != nil {
			t.Error(err)
		}
	}
}

func TestStreamingAndSQLiteResume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "PROTOTYPE-session.sqlite")
	svc, closeDB := databaseService(t, path)
	id := createSession(t, svc)
	llm := &scriptedModel{t: t, respond: func(_ int, _ *model.LLMRequest) []*model.LLMResponse {
		return []*model.LLMResponse{text("Hello ", true), text("ADK", true), text("Hello ADK", false)}
	}}
	r := makeRunner(t, svc, llm)
	var chunks []string
	finalSeen := false
	for e, err := range r.Run(t.Context(), "user", id, genai.NewContentFromText("Remember mango-742", genai.RoleUser), agent.RunConfig{StreamingMode: agent.StreamingModeSSE}) {
		if err != nil {
			t.Fatal(err)
		}
		if e.Content == nil {
			continue
		}
		if e.Partial {
			if finalSeen {
				t.Fatal("partial text arrived after final event")
			}
			for _, p := range e.Content.Parts {
				chunks = append(chunks, p.Text)
			}
		} else {
			finalSeen = true
		}
	}
	if !reflect.DeepEqual(chunks, []string{"Hello ", "ADK"}) || !finalSeen {
		t.Fatalf("chunks=%q final=%v", chunks, finalSeen)
	}
	stored := events(t, svc, id)
	if stored.Events().Len() != 2 {
		t.Fatalf("expected user + final response, got %d events", stored.Events().Len())
	}
	for e := range stored.Events().All() {
		if e.Partial || e.ID == "" {
			t.Fatal("stored partial or missing stable event ID")
		}
	}
	closeDB()

	// Close the original connection and construct a new service and runner.
	// This proves on-disk reload, not process-crash recovery.
	svc, closeDB = databaseService(t, path)
	defer closeDB()
	stored = events(t, svc, id)
	budget, err := stored.State().Get("budget")
	if err != nil || fmt.Sprint(budget) != "32000" {
		t.Fatalf("state did not survive: %v %v", budget, err)
	}
	resumed := &scriptedModel{t: t, respond: func(_ int, req *model.LLMRequest) []*model.LLMResponse {
		content := requestText(req)
		if !strings.Contains(content, "mango-742") || !strings.Contains(content, "Hello ADK") {
			t.Fatal("resumed model did not receive prior history")
		}
		return []*model.LLMResponse{text("mango-742", false)}
	}}
	run(t, t.Context(), makeRunner(t, svc, resumed), id, "What was the code?")
	if events(t, svc, id).Events().Len() != 4 {
		t.Fatal("resumed events not appended to same session")
	}
	t.Log("PASS: two streamed deltas, final-only history, SQLite reload, settings, same-session continuation")
}

type readArgs struct {
	Path  string `json:"path"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}
type readResult struct {
	Text  string `json:"text"`
	Error string `json:"error,omitempty"`
	More  bool   `json:"more"`
}

func TestBoundedReadToolAndSequentialExecution(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "notes.txt"), []byte("first\nsecond\nthird\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "huge.txt"), []byte(strings.Repeat("x", 5000)), 0600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("OUTSIDE"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(project, "escape")); err != nil {
		t.Fatal(err)
	}
	var order []string
	reader, err := functiontool.New(functiontool.Config{Name: "read_file", Description: "Read a bounded project line range"},
		func(_ agent.Context, args readArgs) (readResult, error) {
			order = append(order, args.Path)
			if args.Start < 1 || args.End < args.Start {
				return readResult{Error: "invalid range"}, nil
			}
			f, err := os.OpenInRoot(project, args.Path)
			if err != nil {
				return readResult{Error: "outside scope or unavailable"}, nil
			}
			defer f.Close()
			// Prototype cap, not the planned production continuation contract.
			data, err := io.ReadAll(io.LimitReader(f, 4097))
			if err != nil {
				return readResult{}, err
			}
			if len(data) > 4096 {
				return readResult{Error: "byte limit exceeded; production needs continuation", More: true}, nil
			}
			lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
			var b strings.Builder
			for i := args.Start; i <= args.End && i <= len(lines); i++ {
				fmt.Fprintf(&b, "%d: %s\n", i, lines[i-1])
			}
			return readResult{Text: b.String(), More: args.End < len(lines)}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"notes.txt", "escape/secret.txt", "huge.txt"}
	llm := &scriptedModel{t: t, respond: func(step int, req *model.LLMRequest) []*model.LLMResponse {
		if step == 1 {
			var parts []*genai.Part
			for i, path := range paths {
				parts = append(parts, &genai.Part{FunctionCall: &genai.FunctionCall{ID: fmt.Sprint(i), Name: "read_file", Args: map[string]any{"path": path, "start": 2, "end": 2}}})
			}
			return []*model.LLMResponse{{Content: &genai.Content{Role: "model", Parts: parts}}}
		}
		responses := map[string]map[string]any{}
		for _, c := range req.Contents {
			for _, p := range c.Parts {
				if p.FunctionResponse != nil {
					responses[p.FunctionResponse.ID] = p.FunctionResponse.Response
				}
			}
		}
		if len(responses) != 3 {
			t.Fatalf("tool result IDs: %v", responses)
		}
		if responses["0"]["text"] != "2: second\n" {
			t.Fatalf("range read: %v", responses["0"])
		}
		if responses["1"]["error"] == "" || responses["1"]["text"] == "OUTSIDE" {
			t.Fatal("external symlink not denied")
		}
		if responses["2"]["more"] != true {
			t.Fatal("oversized line not bounded")
		}
		return []*model.LLMResponse{text("Read the second line safely.", false)}
	}}
	svc := session.InMemoryService()
	id := createSession(t, svc)
	serialUsed := false
	ctx := platform.WithTaskRunner(t.Context(), func(ctx context.Context, tasks []func(context.Context)) {
		serialUsed = true
		for _, task := range tasks {
			task(ctx)
		}
	})
	run(t, ctx, makeRunner(t, svc, llm, reader), id, "Read the requested files")
	if !serialUsed || !reflect.DeepEqual(order, paths) {
		t.Fatalf("sequential hook=%v order=%v", serialUsed, order)
	}
	t.Log("PASS: ADK executes calls and returns results; our wrapper enforces ranges, size, symlink scope; explicit hook enforces sequential order")
}

func TestBeforeModelHookCanBlockRequest(t *testing.T) {
	called := false
	llm := &scriptedModel{t: t, respond: func(_ int, _ *model.LLMRequest) []*model.LLMResponse { called = true; return nil }}
	blocked := errors.New("prototype context budget exceeded")
	a, err := llmagent.New(llmagent.Config{Name: "prototype", Model: llm, BeforeModelCallbacks: []llmagent.BeforeModelCallback{
		func(_ agent.Context, _ *model.LLMRequest) (*model.LLMResponse, error) { return nil, blocked },
	}})
	if err != nil {
		t.Fatal(err)
	}
	svc := session.InMemoryService()
	r, err := runner.New(runner.Config{AppName: "prototype", Agent: a, SessionService: svc})
	if err != nil {
		t.Fatal(err)
	}
	seenError := false
	for _, err := range r.Run(t.Context(), "user", createSession(t, svc), genai.NewContentFromText("hello", genai.RoleUser), agent.RunConfig{}) {
		if err != nil {
			seenError = strings.Contains(err.Error(), blocked.Error())
		}
	}
	if called || !seenError {
		t.Fatalf("model called=%v budget error=%v", called, seenError)
	}
	t.Log("PASS: before-model callback blocks request; this does not implement token accounting or automatic compaction")
}

func TestOpenAIResponsesAdapterWithLocalHTTPStub(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/responses" {
			t.Errorf("unexpected route %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []map[string]any{
			{"type": "response.created", "response": map[string]any{"id": "resp_local", "status": "in_progress"}},
			{"type": "response.output_text.delta", "delta": "Hello "},
			{"type": "response.output_text.delta", "delta": "stub"},
			{"type": "response.completed", "response": map[string]any{"id": "resp_local", "status": "completed", "output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "Hello stub"}}}}, "usage": map[string]any{"input_tokens": 12, "output_tokens": 3, "total_tokens": 15}}},
		} {
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event["type"], data)
			w.(http.Flusher).Flush()
		}
	}))
	defer server.Close()
	llm, err := openaimodel.NewModel(t.Context(), "gpt-4o-mini", &openaimodel.ClientConfig{APIKey: "local-test-only", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	var chunks []string
	var usage *genai.GenerateContentResponseUsageMetadata
	for response, err := range llm.GenerateContent(t.Context(), &model.LLMRequest{Contents: []*genai.Content{genai.NewContentFromText("hi", genai.RoleUser)}, Config: &genai.GenerateContentConfig{MaxOutputTokens: 123}}, true) {
		if err != nil {
			t.Fatal(err)
		}
		if response.Partial && response.Content != nil {
			for _, p := range response.Content.Parts {
				chunks = append(chunks, p.Text)
			}
		}
		if response.UsageMetadata != nil {
			usage = response.UsageMetadata
		}
	}
	if strings.Join(chunks, "") != "Hello stub" {
		t.Fatalf("streamed text=%q", chunks)
	}
	if received["max_output_tokens"] != float64(123) || received["stream"] != true {
		t.Fatalf("output limit or streaming missing: %v", received)
	}
	if usage == nil || usage.PromptTokenCount != 12 || usage.CandidatesTokenCount != 3 {
		t.Fatalf("usage=%v", usage)
	}
	t.Log("PASS: ADK OpenAI adapter streams Responses SSE, sends max_output_tokens, maps usage; no live-provider claim")
}

// An injected summarizer tests ADK mechanics, not semantic summary quality.
type fixedSummary struct{ calls int }

func (s *fixedSummary) SummarizeEvents(_ context.Context, source []*session.Event) (compaction.SummarizeResult, error) {
	s.calls++
	return compaction.SummarizeResult{Content: genai.NewContentFromText("Summary: original marker retained.", genai.RoleUser)}, nil
}

func TestCompactionPreservesOriginalSQLiteEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "PROTOTYPE-compaction.sqlite")
	svc, closeDB := databaseService(t, path)
	id := createSession(t, svc)
	llm := &scriptedModel{t: t, respond: func(step int, req *model.LLMRequest) []*model.LLMResponse {
		if step == 2 {
			content := requestText(req)
			if !strings.Contains(content, "Summary:") {
				t.Fatalf("summary not used in next prompt: %s", content)
			}
		}
		return []*model.LLMResponse{text("Reply", false)}
	}}
	a, err := llmagent.New(llmagent.Config{Name: "prototype", Model: llm})
	if err != nil {
		t.Fatal(err)
	}
	summary := &fixedSummary{}
	r, err := runner.New(runner.Config{AppName: "prototype", Agent: a, SessionService: svc, Compaction: &compaction.Config{CompactionInterval: 1, Summarizer: summary}})
	if err != nil {
		t.Fatal(err)
	}
	run(t, t.Context(), r, id, "original marker: raw detail")
	run(t, t.Context(), r, id, "continue")
	if summary.calls == 0 {
		t.Fatal("summarizer never invoked")
	}
	closeDB()
	svc, closeDB = databaseService(t, path)
	defer closeDB()
	original, records := false, 0
	for e := range events(t, svc, id).Events().All() {
		if e.Actions.Compaction != nil {
			records++
		}
		if e.Content != nil {
			for _, p := range e.Content.Parts {
				if p.Text == "original marker: raw detail" {
					original = true
				}
			}
		}
	}
	if !original || records == 0 {
		t.Fatalf("original=%v persisted compaction records=%d", original, records)
	}
	t.Log("PASS: ADK summary changes model context while raw events and compaction records survive database reopen; not our percentage policy")
}

func TestPartialFailureNeedsCustomTranscriptCapture(t *testing.T) {
	svc := session.InMemoryService()
	id := createSession(t, svc)
	llm := &scriptedModel{t: t, fail: errors.New("injected stream disconnect"), respond: func(_ int, _ *model.LLMRequest) []*model.LLMResponse {
		return []*model.LLMResponse{text("visible but unfinished", true)}
	}}
	r := makeRunner(t, svc, llm)
	partial, failure := false, false
	for e, err := range r.Run(t.Context(), "user", id, genai.NewContentFromText("hello", genai.RoleUser), agent.RunConfig{StreamingMode: agent.StreamingModeSSE}) {
		if err != nil {
			failure = true
			continue
		}
		if e.Partial {
			partial = true
		}
	}
	stored := events(t, svc, id)
	if !partial || !failure || stored.Events().Len() != 1 {
		t.Fatalf("partial=%v failure=%v stored=%d", partial, failure, stored.Events().Len())
	}
	t.Log("GAP CONFIRMED: partial text reached consumer but only user input was retained; S01 needs custom partial/failure transcript capture")
}
