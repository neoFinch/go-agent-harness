// PROTOTYPE: a harness-owned capture layer around ADK, not production code.
package adkprototype

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/genai"
)

// CapturedTurn is the user-visible transcript view, separate from ADK history.
// A begin record without a terminal record is recovered as incomplete.
type CapturedTurn struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Prompt    string `json:"prompt,omitempty"`
	Text      string `json:"text,omitempty"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

type captureRecord struct {
	Kind string       `json:"kind"`
	Turn CapturedTurn `json:"turn"`
}

// LoadTranscript reopens the journal and reconstructs captured turns. A corrupt
// or torn JSON record is reported, not silently treated as a complete transcript.
func LoadTranscript(path string) ([]CapturedTurn, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	var turns []CapturedTurn
	positions := map[string]int{}
	for {
		var record captureRecord
		if err := decoder.Decode(&record); err != nil {
			if errors.Is(err, io.EOF) {
				return turns, nil
			}
			return nil, fmt.Errorf("read capture journal: %w", err)
		}
		if record.Turn.ID == "" {
			return nil, errors.New("capture record has no turn ID")
		}
		i, exists := positions[record.Turn.ID]
		if record.Kind == "begin" {
			if exists {
				return nil, errors.New("duplicate captured turn")
			}
			positions[record.Turn.ID] = len(turns)
			turns = append(turns, record.Turn)
			continue
		}
		if !exists || turns[i].SessionID != record.Turn.SessionID {
			return nil, errors.New("capture record has no matching turn")
		}
		switch record.Kind {
		case "chunk":
			turns[i].Text += record.Turn.Text
		case "finish":
			turns[i].Status, turns[i].Error = record.Turn.Status, record.Turn.Error
		default:
			return nil, fmt.Errorf("unknown capture record %q", record.Kind)
		}
	}
}

func appendCapture(path, kind string, turn CapturedTurn) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open capture journal: %w", err)
	}
	writeErr := json.NewEncoder(f).Encode(captureRecord{Kind: kind, Turn: turn})
	if writeErr == nil {
		writeErr = f.Sync()
	}
	return errors.Join(writeErr, f.Close())
}

// RunCaptured runs one turn synchronously. The caller owns the per-session
// journal path and must not run concurrent writers. Only visible text is saved;
// no tool arguments or hidden reasoning are promoted to display text.
func RunCaptured(ctx context.Context, r *runner.Runner, userID, sessionID, prompt, path string, onText func(string)) error {
	turn := CapturedTurn{ID: rand.Text(), SessionID: sessionID, Prompt: prompt, Status: "incomplete"}
	if err := appendCapture(path, "begin", turn); err != nil {
		return err
	}
	finish := func(cause error) error {
		record := CapturedTurn{ID: turn.ID, SessionID: sessionID, Status: "completed"}
		if cause != nil {
			record.Status, record.Error = "failed", cause.Error()
			if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
				record.Status = "cancelled"
			}
		}
		return errors.Join(cause, appendCapture(path, "finish", record))
	}
	emit := func(value string) error {
		if value == "" {
			return nil
		}
		if err := appendCapture(path, "chunk", CapturedTurn{ID: turn.ID, SessionID: sessionID, Text: value}); err != nil {
			return err
		}
		if onText != nil {
			onText(value)
		}
		return nil
	}
	if err := ctx.Err(); err != nil {
		return finish(err)
	}
	partialForResponse, finalSeen := false, false
	for event, err := range r.Run(ctx, userID, sessionID, genai.NewContentFromText(prompt, genai.RoleUser), agent.RunConfig{StreamingMode: agent.StreamingModeSSE}) {
		if err != nil {
			return finish(err)
		}
		if err := ctx.Err(); err != nil {
			return finish(err)
		}
		if event.Content != nil {
			for _, part := range event.Content.Parts {
				if part.Thought || part.Text == "" {
					continue
				}
				if event.Partial || !partialForResponse {
					if err := emit(part.Text); err != nil {
						return finish(err)
					}
				}
			}
		}
		if event.Partial {
			partialForResponse = true
		} else {
			partialForResponse = false
		}
		if event.IsFinalResponse() {
			finalSeen = true
		}
		if err := ctx.Err(); err != nil {
			return finish(err)
		}
	}
	if !finalSeen {
		return finish(errors.New("stream ended without a final answer"))
	}
	return finish(nil)
}
