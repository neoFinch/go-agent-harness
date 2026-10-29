package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/neoFinch/agent-harness/internal/harness"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/model/openaimodel"
	"google.golang.org/genai"
)

func TestCLIUsesADKOpenAIStreamingAndPersistsConversation(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" || r.Method != "POST" {
			t.Errorf("unexpected route %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			http.Error(w, "bad request", 400)
			return
		}
		if body["model"] != "gpt-4o-mini" || body["stream"] != true {
			t.Errorf("wrong model or non-streaming request: %v", body)
		}
		serialized, _ := json.Marshal(body["input"])
		mu.Lock()
		requests = append(requests, string(serialized))
		step := len(requests)
		mu.Unlock()
		if step == 2 && (!strings.Contains(string(serialized), "remember neoFinch") || !strings.Contains(string(serialized), "Hi neoFinch.")) {
			t.Error("follow-up lost context")
		}
		if step >= 3 && strings.Contains(string(serialized), "neoFinch") {
			t.Error("/clear did not reset model context")
		}
		response := []string{"Hi neoFinch.", "Your name is neoFinch.", "Fresh start.", "Resumed fresh context."}
		if step > len(response) {
			t.Error("unexpected model call")
			http.Error(w, "extra call", 400)
			return
		}
		answer := response[step-1]
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []map[string]any{
			{"type": "response.created", "response": map[string]any{"id": fmt.Sprintf("resp_%d", step), "status": "in_progress"}},
			{"type": "response.output_text.delta", "delta": answer[:3]},
			{"type": "response.output_text.delta", "delta": answer[3:]},
			{"type": "response.completed", "response": map[string]any{"id": fmt.Sprintf("resp_%d", step), "status": "completed", "output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": answer}}}}}},
		} {
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event["type"], data)
			w.(http.Flusher).Flush()
		}
	}))
	defer server.Close()
	t.Setenv("OPENAI_API_KEY", "offline-test-key")
	t.Setenv("OPENAI_BASE_URL", server.URL)
	t.Setenv("OPENAI_MODEL", "")
	dir := t.TempDir()
	var out, errOut bytes.Buffer
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	err := runCLI(ctx, []string{"-data-dir", dir}, strings.NewReader("\nremember neoFinch\nwhat name?\n/clear\nfresh\nexit\n"), &out, &errOut, nil)
	if err != nil {
		t.Fatal(err)
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr=%s", errOut.String())
	}
	for _, want := range []string{"Assistant: Hi neoFinch.", "Assistant: Your name is neoFinch.", "Active context cleared. Saved transcript retained.", "Assistant: Fresh start."} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %s", want, out.String())
		}
	}
	mu.Lock()
	count := len(requests)
	mu.Unlock()
	if count != 3 {
		t.Fatalf("expected 3 requests, got %d", count)
	}
	matches := regexp.MustCompile(`Session: (\S+)`).FindStringSubmatch(out.String())
	if len(matches) != 2 {
		t.Fatalf("session ID not shown: %s", out.String())
	}
	// Even with another startup default, resume must reconstruct the saved
	// official provider adapter and keep the saved clear boundary.
	var resumedOut, resumedErr bytes.Buffer
	if err := runCLI(ctx, []string{"-data-dir", dir, "-model", "different-startup-default", "-resume", matches[1]}, strings.NewReader("continue the fresh conversation\nexit\n"), &resumedOut, &resumedErr, nil); err != nil {
		t.Fatal(err)
	}
	if resumedErr.Len() != 0 || !strings.Contains(resumedOut.String(), "Model: gpt-4o-mini") || !strings.Contains(resumedOut.String(), "Assistant: Resumed fresh context.") {
		t.Fatalf("resume output=%s errors=%s", resumedOut.String(), resumedErr.String())
	}
	llm, err := openaimodel.NewModel(t.Context(), "gpt-4o-mini", nil)
	if err != nil {
		t.Fatal(err)
	}
	inspector, err := harness.Open(t.Context(), harness.Config{DataDir: dir, Model: llm})
	if err != nil {
		t.Fatal(err)
	}
	defer inspector.Close()
	turns, err := inspector.Transcript(t.Context(), matches[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 4 || turns[0].Text != "Hi neoFinch." || turns[3].Status != "completed" {
		t.Fatalf("saved CLI turns=%+v", turns)
	}
	if !strings.Contains(out.String(), filepath.Join(dir, "sessions.sqlite")) {
		t.Fatal("storage path not shown")
	}
}

func TestCLINewListAndResumeAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	llm := &cliSessionModel{}
	var first, second, errs bytes.Buffer
	if err := runCLI(t.Context(), []string{"-data-dir", dir}, strings.NewReader("remember 3412\nexit\n"), &first, &errs, llm); err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`Session: (\S+)`).FindStringSubmatch(first.String())
	if len(match) != 2 {
		t.Fatal(first.String())
	}
	id := match[1]
	input := "/help\n/sessions\nwhat was the code?\n/new\nfresh request\n/resume missing\n/resume " + id + "\ncontinue\nexit\n"
	if err := runCLI(t.Context(), []string{"-data-dir", dir, "-resume", id}, strings.NewReader(input), &second, &errs, llm); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errs.String(), "session not found") {
		t.Fatalf("missing actionable error: %s", errs.String())
	}
	if len(llm.inputs) != 4 {
		t.Fatalf("commands sent to model: %v", llm.inputs)
	}
	if !strings.Contains(llm.inputs[1], "remember 3412") || strings.Contains(llm.inputs[2], "3412") || !strings.Contains(llm.inputs[3], "remember 3412") {
		t.Fatalf("session switching lost isolation: %v", llm.inputs)
	}
	for _, want := range []string{"/new", "/sessions", "/resume <id>", "Resumed session: " + id, "New session:"} {
		if !strings.Contains(second.String(), want) {
			t.Fatalf("missing %q in %s", want, second.String())
		}
	}
	inspector, err := harness.Open(t.Context(), harness.Config{DataDir: dir, Model: llm, SessionID: id})
	if err != nil {
		t.Fatal(err)
	}
	defer inspector.Close()
	list, err := inspector.Sessions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("resume created extra sessions: %v", list)
	}
	turns, err := inspector.Transcript(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 3 {
		t.Fatalf("wrong resumed transcript: %+v", turns)
	}
}

type cliSessionModel struct{ inputs []string }

func (*cliSessionModel) Name() string { return "offline-cli" }
func (m *cliSessionModel) GenerateContent(_ context.Context, req *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		var input strings.Builder
		for _, content := range req.Contents {
			for _, part := range content.Parts {
				input.WriteString(part.Text)
			}
		}
		m.inputs = append(m.inputs, input.String())
		yield(&model.LLMResponse{Content: genai.NewContentFromText("saved", genai.RoleModel), TurnComplete: true}, nil)
	}
}
