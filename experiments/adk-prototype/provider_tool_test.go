package adkprototype

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model/openaimodel"
	"google.golang.org/adk/v2/platform"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool/functiontool"
)

func encodeSSE(events ...map[string]any) string {
	var b strings.Builder
	for _, event := range events {
		data, _ := json.Marshal(event)
		fmt.Fprintf(&b, "event: %s\ndata: %s\n\n", event["type"], data)
	}
	return b.String()
}

func TestOpenAIStreamedToolRoundTrip(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "notes.txt"), []byte("first\nsecond\nthird\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var toolCalls atomic.Int32
	var actualArgs readArgs
	reader, err := functiontool.New(functiontool.Config{Name: "read_file", Description: "Read a project file line range"}, func(_ agent.Context, args readArgs) (readResult, error) {
		toolCalls.Add(1)
		actualArgs = args
		if args.Start < 1 || args.End < args.Start {
			return readResult{}, fmt.Errorf("invalid line range")
		}
		file, err := os.OpenInRoot(project, args.Path)
		if err != nil {
			return readResult{}, err
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, 4097))
		if err != nil {
			return readResult{}, err
		}
		if len(data) > 4096 {
			return readResult{}, fmt.Errorf("fixture exceeds byte cap")
		}
		lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
		var result strings.Builder
		for i := args.Start; i <= args.End && i <= len(lines); i++ {
			fmt.Fprintf(&result, "%d: %s\n", i, lines[i-1])
		}
		return readResult{Text: result.String(), More: args.End < len(lines)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/responses" {
			t.Errorf("route=%s %s", r.Method, r.URL.Path)
			http.Error(w, "wrong route", 400)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			http.Error(w, "bad request", 400)
			return
		}
		mu.Lock()
		requests = append(requests, body)
		number := len(requests)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		if number == 1 {
			io.WriteString(w, encodeSSE(
				map[string]any{"type": "response.created", "response": map[string]any{"id": "resp_tool", "status": "in_progress"}},
				map[string]any{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{"type": "function_call", "id": "fc_read", "call_id": "call_read_742", "name": "read_file", "arguments": ""}},
				map[string]any{"type": "response.function_call_arguments.delta", "item_id": "fc_read", "delta": "{\"path\":\"no"},
				map[string]any{"type": "response.function_call_arguments.delta", "item_id": "fc_read", "delta": "tes.txt\",\"start\":"},
				map[string]any{"type": "response.function_call_arguments.delta", "item_id": "fc_read", "delta": "2,\"end\":2}"},
				// Empty done arguments forces the adapter to assemble the actual deltas.
				map[string]any{"type": "response.function_call_arguments.done", "item_id": "fc_read", "arguments": ""},
				map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp_tool", "status": "completed", "output": []any{}}},
			))
		} else if number == 2 {
			io.WriteString(w, encodeSSE(
				map[string]any{"type": "response.created", "response": map[string]any{"id": "resp_answer", "status": "in_progress"}},
				map[string]any{"type": "response.output_text.delta", "delta": "The second "},
				map[string]any{"type": "response.output_text.delta", "delta": "line is second."},
				map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp_answer", "status": "completed", "output": []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "The second line is second."}}}}}},
			))
		} else {
			t.Errorf("unexpected extra model request %d", number)
			io.WriteString(w, encodeSSE(map[string]any{"type": "error", "message": "unexpected extra request"}))
		}
		w.(http.Flusher).Flush()
	}))
	defer server.Close()
	llm, err := openaimodel.NewModel(t.Context(), "gpt-4o-mini", &openaimodel.ClientConfig{APIKey: "local-test-only", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	svc := session.InMemoryService()
	id := createSession(t, svc)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ctx = platform.WithTaskRunner(ctx, func(ctx context.Context, tasks []func(context.Context)) {
		for _, task := range tasks {
			task(ctx)
		}
	})
	var chunks []string
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	err = RunCaptured(ctx, makeRunner(t, svc, llm, reader), "user", id, "Read line 2 of notes.txt", path, func(s string) { chunks = append(chunks, s) })
	if err != nil {
		t.Fatal(err)
	}
	if toolCalls.Load() != 1 || actualArgs != (readArgs{Path: "notes.txt", Start: 2, End: 2}) {
		t.Fatalf("executed %d times; args=%+v", toolCalls.Load(), actualArgs)
	}
	if !reflect.DeepEqual(chunks, []string{"The second ", "line is second."}) {
		t.Fatalf("answer chunks=%q", chunks)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("model requests=%d", len(requests))
	}
	tools, ok := requests[0]["tools"].([]any)
	if !ok || len(tools) != 1 || tools[0].(map[string]any)["name"] != "read_file" {
		t.Fatalf("tool schema not advertised: %v", requests[0]["tools"])
	}
	input, ok := requests[1]["input"].([]any)
	if !ok {
		t.Fatalf("next request input=%v", requests[1]["input"])
	}
	calls, results := 0, 0
	for _, raw := range input {
		item := raw.(map[string]any)
		switch item["type"] {
		case "function_call":
			calls++
			if item["call_id"] != "call_read_742" {
				t.Fatalf("call ID=%v", item["call_id"])
			}
		case "function_call_output":
			results++
			if item["call_id"] != "call_read_742" {
				t.Fatalf("result call ID=%v", item["call_id"])
			}
			var result map[string]any
			if err := json.Unmarshal([]byte(item["output"].(string)), &result); err != nil {
				t.Fatal(err)
			}
			if result["text"] != "2: second\n" {
				t.Fatalf("wrong real file result in follow-up request: %v", result)
			}
		}
	}
	if calls != 1 || results != 1 {
		t.Fatalf("call/result counts=%d/%d", calls, results)
	}
	turns, err := LoadTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Text != "The second line is second." || turns[0].Status != "completed" {
		t.Fatalf("capture=%+v", turns)
	}
	t.Log("PASS: fragmented JSON -> one real file read -> matching call/result ID -> follow-up request -> two streamed answer chunks")
}

func TestOpenAIToolStreamFailuresDoNotExecuteTools(t *testing.T) {
	for _, mode := range []string{"malformed_arguments", "connection_cut_mid_arguments"} {
		t.Run(mode, func(t *testing.T) {
			var executions, requests atomic.Int32
			reader, err := functiontool.New(functiontool.Config{Name: "read_file", Description: "Must not execute an incomplete tool request"}, func(_ agent.Context, _ readArgs) (readResult, error) {
				executions.Add(1)
				return readResult{Text: "should never run"}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				payload := encodeSSE(
					map[string]any{"type": "response.created", "response": map[string]any{"id": "resp_broken", "status": "in_progress"}},
					map[string]any{"type": "response.output_item.added", "item": map[string]any{"type": "function_call", "id": "fc_broken", "call_id": "call_broken", "name": "read_file", "arguments": ""}},
					map[string]any{"type": "response.function_call_arguments.delta", "item_id": "fc_broken", "delta": "{\"path\":"},
				)
				if mode == "malformed_arguments" {
					payload += encodeSSE(map[string]any{"type": "response.function_call_arguments.done", "item_id": "fc_broken", "arguments": ""})
				} else {
					// A larger promised body than delivered creates a real HTTP unexpected EOF,
					// rather than pretending that a clean end-of-stream is a network failure.
					w.Header().Set("Content-Length", fmt.Sprint(len(payload)+1000))
				}
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, payload)
				w.(http.Flusher).Flush()
			}))
			defer server.Close()
			llm, err := openaimodel.NewModel(t.Context(), "gpt-4o-mini", &openaimodel.ClientConfig{APIKey: "local-test-only", BaseURL: server.URL, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			svc := session.InMemoryService()
			id := createSession(t, svc)
			path := filepath.Join(t.TempDir(), "transcript.jsonl")
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			var visible strings.Builder
			err = RunCaptured(ctx, makeRunner(t, svc, llm, reader), "user", id, "Read the file", path, func(s string) { visible.WriteString(s) })
			if err == nil {
				t.Fatal("broken stream unexpectedly succeeded")
			}
			if executions.Load() != 0 || requests.Load() != 1 {
				t.Fatalf("executions=%d model requests=%d error=%v", executions.Load(), requests.Load(), err)
			}
			if visible.Len() != 0 {
				t.Fatalf("tool arguments leaked to display: %q", visible.String())
			}
			turns, loadErr := LoadTranscript(path)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if len(turns) != 1 || turns[0].Status != "failed" || turns[0].Error == "" {
				t.Fatalf("failure not captured: %+v", turns)
			}
			if events(t, svc, id).Events().Len() != 1 {
				t.Fatal("incomplete tool call was saved as a completed event")
			}
			t.Logf("PASS: no tool executed, no follow-up model request; failure captured (%v)", err)
		})
	}
}
