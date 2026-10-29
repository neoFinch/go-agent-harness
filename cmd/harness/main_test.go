package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/neoFinch/agent-harness/internal/harness"
	"google.golang.org/adk/v2/model/openaimodel"
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
		if step == 3 && strings.Contains(string(serialized), "neoFinch") {
			t.Error("/clear did not reset model context")
		}
		response := []string{"Hi neoFinch.", "Your name is neoFinch.", "Fresh start."}
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
	if len(turns) != 3 || turns[0].Text != "Hi neoFinch." || turns[2].Status != "completed" {
		t.Fatalf("saved CLI turns=%+v", turns)
	}
	if !strings.Contains(out.String(), filepath.Join(dir, "sessions.sqlite")) {
		t.Fatal("storage path not shown")
	}
}
