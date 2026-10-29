package adkprototype

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
)

func TestCapturedPartialFailureSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	svc := session.InMemoryService()
	id := createSession(t, svc)
	disconnect := errors.New("injected stream disconnect")
	llm := &scriptedModel{t: t, fail: disconnect, respond: func(_ int, _ *model.LLMRequest) []*model.LLMResponse {
		return []*model.LLMResponse{text("visible ", true), text("but unfinished", true)}
	}}
	var visible strings.Builder
	err := RunCaptured(t.Context(), makeRunner(t, svc, llm), "user", id, "keep my input", path, func(chunk string) {
		visible.WriteString(chunk)
		// Each chunk must already be durable when it is shown.
		turns, loadErr := LoadTranscript(path)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		if len(turns) != 1 || turns[0].Text != visible.String() {
			t.Fatalf("visible output not saved: %+v", turns)
		}
	})
	if err == nil || !strings.Contains(err.Error(), disconnect.Error()) {
		t.Fatalf("stream failure lost: %v", err)
	}
	turns, err := LoadTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 {
		t.Fatalf("got %d saved turns", len(turns))
	}
	turn := turns[0]
	if turn.ID == "" || turn.SessionID != id || turn.Prompt != "keep my input" || turn.Text != "visible but unfinished" || turn.Status != "failed" || turn.Error == "" {
		t.Fatalf("incomplete transcript not recoverable: %+v", turn)
	}
	if events(t, svc, id).Events().Len() != 1 {
		t.Fatal("unfinished answer leaked into successful ADK history")
	}
}

func TestCapturedSuccessfulTurnsDoNotDuplicateFinalText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	svc := session.InMemoryService()
	id := createSession(t, svc)
	llm := &scriptedModel{t: t, respond: func(step int, _ *model.LLMRequest) []*model.LLMResponse {
		if step == 1 {
			return []*model.LLMResponse{text("hello ", true), text("world", true), text("hello world", false)}
		}
		return []*model.LLMResponse{text("final-only reply", false)}
	}}
	r := makeRunner(t, svc, llm)
	var visible strings.Builder
	for _, prompt := range []string{"first", "second"} {
		if err := RunCaptured(t.Context(), r, "user", id, prompt, path, func(chunk string) { visible.WriteString(chunk) }); err != nil {
			t.Fatal(err)
		}
	}
	turns, err := LoadTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[0].Status != "completed" || turns[1].Status != "completed" || turns[0].Text != "hello world" || turns[1].Text != "final-only reply" || turns[0].ID == turns[1].ID {
		t.Fatalf("turns=%+v", turns)
	}
	if visible.String() != "hello worldfinal-only reply" {
		t.Fatalf("duplicated output: %q", visible.String())
	}
}

func TestCapturedCancellationPreservesPartialText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	svc := session.InMemoryService()
	id := createSession(t, svc)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	llm := &scriptedModel{t: t, respond: func(_ int, _ *model.LLMRequest) []*model.LLMResponse {
		return []*model.LLMResponse{text("first chunk", true), text("never displayed", true)}
	}}
	err := RunCaptured(ctx, makeRunner(t, svc, llm), "user", id, "hello", path, func(_ string) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	turns, err := LoadTranscript(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Status != "cancelled" || turns[0].Text != "first chunk" {
		t.Fatalf("turns=%+v", turns)
	}
}

func TestCaptureStorageFailurePreventsGeneration(t *testing.T) {
	svc := session.InMemoryService()
	id := createSession(t, svc)
	llm := &scriptedModel{t: t, respond: func(_ int, _ *model.LLMRequest) []*model.LLMResponse {
		t.Fatal("model invoked before input saved")
		return nil
	}}
	err := RunCaptured(t.Context(), makeRunner(t, svc, llm), "user", id, "hello", t.TempDir(), nil)
	if err == nil || llm.step != 0 {
		t.Fatalf("storage failure not surfaced: %v", err)
	}
}

func TestCaptureWriteFailureDoesNotDisplayUnsavedChunk(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "transcript.jsonl")
	saved := filepath.Join(folder, "before-failure.jsonl")
	svc := session.InMemoryService()
	id := createSession(t, svc)
	llm := &scriptedModel{t: t, respond: func(_ int, _ *model.LLMRequest) []*model.LLMResponse {
		return []*model.LLMResponse{text("saved", true), text("not saved", true)}
	}}
	var visible strings.Builder
	err := RunCaptured(t.Context(), makeRunner(t, svc, llm), "user", id, "hello", path, func(chunk string) {
		visible.WriteString(chunk)
		if err := os.Rename(path, saved); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	})
	if err == nil || visible.String() != "saved" {
		t.Fatalf("err=%v output=%q", err, visible.String())
	}
	turns, err := LoadTranscript(saved)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Status != "incomplete" || turns[0].Text != "saved" {
		t.Fatalf("recoverable turns=%+v", turns)
	}
}
