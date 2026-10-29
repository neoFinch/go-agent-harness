// PROTOTYPE: interactive, offline illustration of ADK stream capture.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"iter"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	capture "agent-harness/adk-prototype"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

// A scripted model lets us reproduce a disconnect without an API key or cost.
type demoModel struct{ delay time.Duration }

func (*demoModel) Name() string { return "scripted-demo-not-a-live-model" }
func (m *demoModel) GenerateContent(ctx context.Context, request *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		fail := false
		for i := len(request.Contents) - 1; i >= 0; i-- {
			if request.Contents[i].Role != "user" {
				continue
			}
			for _, p := range request.Contents[i].Parts {
				if p.Text == "Simulate a disconnect" {
					fail = true
				}
			}
			break
		}
		words := []string{"Hello! ", "These words ", "are streamed ", "through ADK ", "and saved ", "before display."}
		if fail {
			words = []string{"This reply ", "started normally, ", "but the connection ", "will fail here..."}
		}
		var full strings.Builder
		for _, word := range words {
			timer := time.NewTimer(m.delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				yield(nil, ctx.Err())
				return
			case <-timer.C:
			}
			full.WriteString(word)
			if !yield(&model.LLMResponse{Content: genai.NewContentFromText(word, genai.RoleModel), Partial: true}, nil) {
				return
			}
		}
		if fail {
			yield(nil, errors.New("simulated connection lost (intentional demo)"))
			return
		}
		yield(&model.LLMResponse{Content: genai.NewContentFromText(full.String(), genai.RoleModel), TurnComplete: true}, nil)
	}
}

func showTranscript(path string) error {
	turns, err := capture.LoadTranscript(path)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println("No saved turns yet.")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Printf("\nRecovered from disk: %d turn(s)\n", len(turns))
	for i, t := range turns {
		fmt.Printf("\n%d. [%s] turn=%s\n   Session: %s\n   Prompt: %s\n   Saved text: %s\n", i+1, t.Status, t.ID, t.SessionID, t.Prompt, t.Text)
		if t.Error != "" {
			fmt.Printf("   Error: %s\n", t.Error)
		}
	}
	return nil
}

func demo() error {
	transcript := flag.String("transcript", "", "Existing journal to inspect/append; parent directory must exist")
	delay := flag.Duration("delay", 250*time.Millisecond, "Pause between scripted chunks (use 0 for a fast smoke run)")
	flag.Parse()
	if *delay < 0 {
		return errors.New("delay must not be negative")
	}
	if *transcript == "" {
		dir, err := os.MkdirTemp("", "agent-harness-adk-demo-")
		if err != nil {
			return err
		}
		*transcript = filepath.Join(dir, "transcript.jsonl")
	}
	path, err := filepath.Abs(*transcript)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	svc := session.InMemoryService()
	created, err := svc.Create(ctx, &session.CreateRequest{AppName: "capture-demo", UserID: "demo"})
	if err != nil {
		return err
	}
	a, err := llmagent.New(llmagent.Config{Name: "demo_agent", Model: &demoModel{delay: *delay}})
	if err != nil {
		return err
	}
	r, err := runner.New(runner.Config{AppName: "capture-demo", Agent: a, SessionService: svc})
	if err != nil {
		return err
	}
	fmt.Println("ADK stream-capture demo — scripted, offline, no API calls.")
	fmt.Printf("Journal: %s\nSession: %s\n", path, created.Session.ID())
	fmt.Println("The journal stays on disk. Restarting creates a new ADK session; this is not full session resume.")
	fmt.Println("Commands: /success, /fail, /transcript, /exit. Ctrl+C cancels and saves partial text.")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\nDemo> ")
		if !scanner.Scan() {
			break
		}
		command := strings.TrimSpace(scanner.Text())
		if command == "/exit" {
			break
		}
		if command == "" {
			continue
		}
		if command == "/transcript" {
			if err := showTranscript(path); err != nil {
				return err
			}
			continue
		}
		prompt := ""
		switch command {
		case "/success":
			prompt = "Demonstrate a completed stream"
		case "/fail":
			prompt = "Simulate a disconnect"
		default:
			fmt.Println("Use /success, /fail, /transcript, or /exit.")
			continue
		}
		fmt.Print("Assistant: ")
		runErr := capture.RunCaptured(ctx, r, "demo", created.Session.ID(), prompt, path, func(chunk string) { fmt.Print(chunk) })
		fmt.Println()
		if runErr != nil {
			fmt.Printf("Turn stopped: %v\n", runErr)
		}
		if err := showTranscript(path); err != nil {
			return err
		}
		if ctx.Err() != nil {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	fmt.Printf("\nJournal kept at: %s\n", path)
	fmt.Printf("Inspect again: go run ./cmd/demo -transcript %q\n", path)
	return nil
}

func main() {
	if err := demo(); err != nil {
		fmt.Fprintln(os.Stderr, "Demo error:", err)
		os.Exit(1)
	}
}
