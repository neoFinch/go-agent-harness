package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/neoFinch/agent-harness/internal/harness"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/model/openaimodel"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := runCLI(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, nil); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runCLI(ctx context.Context, args []string, input io.Reader, output, errorOutput io.Writer, llm model.LLM) error {
	flags := flag.NewFlagSet("harness", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	defaultModel := os.Getenv("OPENAI_MODEL")
	if defaultModel == "" {
		defaultModel = "gpt-4o-mini"
	}
	modelName := flags.String("model", defaultModel, "OpenAI model name (or OPENAI_MODEL)")
	resumeID := flags.String("resume", "", "Resume a saved session ID (saved model takes precedence)")
	dataDir := flags.String("data-dir", ".harness", "Project-local session data directory")
	flags.Usage = func() {
		fmt.Fprint(errorOutput, "Usage: harness [flags]\n\n")
		flags.PrintDefaults()
		fmt.Fprint(errorOutput, "\n"+commandHelp)
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected arguments; run harness -h for usage")
	}
	var factory func(context.Context, string) (model.LLM, error)
	if llm == nil {
		factory = func(ctx context.Context, name string) (model.LLM, error) { return openaimodel.NewModel(ctx, name, nil) }
		var err error
		// ADK's official adapter reads OPENAI_API_KEY and OPENAI_BASE_URL.
		llm, err = openaimodel.NewModel(ctx, *modelName, nil)
		if err != nil {
			return err
		}
	}
	path, err := filepath.Abs(*dataDir)
	if err != nil {
		return err
	}
	controller, err := harness.Open(ctx, harness.Config{DataDir: path, Model: llm, SessionID: *resumeID, ModelFactory: factory})
	if err != nil {
		return fmt.Errorf("start harness: %w", err)
	}
	defer controller.Close()
	if _, err := fmt.Fprintf(output, "Harness ready. Type /help for commands, exit to quit.\nSession: %s\nSaved in: %s\n", controller.SessionID(), filepath.Join(path, "sessions.sqlite")); err != nil {
		return err
	}
	if *resumeID != "" {
		if err := showSession(output, "Resumed session", controller); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type line struct {
		text string
		err  error
		done bool
	}
	lines := make(chan line)
	go func() {
		scanner := bufio.NewScanner(input)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		for scanner.Scan() {
			select {
			case lines <- line{text: scanner.Text()}:
			case <-ctx.Done():
				return
			}
		}
		select {
		case lines <- line{err: scanner.Err(), done: true}:
		case <-ctx.Done():
		}
	}()
	for {
		if _, err := fmt.Fprint(output, "\nYou: "); err != nil {
			return err
		}
		var next line
		select {
		case <-ctx.Done():
			return nil
		case next = <-lines:
		}
		if next.done {
			if next.err != nil {
				return fmt.Errorf("read prompt: %w", next.err)
			}
			return nil
		}
		prompt := strings.TrimSpace(next.text)
		switch prompt {
		case "":
			continue
		case "exit", "/exit":
			return nil
		}
		if strings.HasPrefix(prompt, "/") {
			if err := runCommand(ctx, prompt, controller, output); err != nil {
				if _, writeErr := fmt.Fprintln(errorOutput, "Command failed:", err); writeErr != nil {
					return writeErr
				}
			}
			continue
		}
		if _, err := fmt.Fprint(output, "Assistant: "); err != nil {
			return err
		}
		turnCtx, stopTurn := context.WithCancel(ctx)
		var writeErr error
		runErr := controller.Run(turnCtx, next.text, func(chunk string) {
			if writeErr == nil {
				_, writeErr = fmt.Fprint(output, chunk)
				if writeErr != nil {
					stopTurn()
				}
			}
		})
		stopTurn()
		if writeErr != nil {
			return fmt.Errorf("write response: %w", writeErr)
		}
		if _, err := fmt.Fprintln(output); err != nil {
			return err
		}
		if runErr != nil {
			fmt.Fprintln(errorOutput, "Turn failed (input and visible output retained when storage is available):", runErr)
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

const commandHelp = `Commands:
  /help           Show this help
  /sessions       List saved sessions in this data directory (* is active)
  /new            Start a new session using the startup model default
  /resume <id>    Resume a saved session and its model/context
  /clear          Reset active context; retain session and saved transcript
  /exit or exit   Exit
`

func showSession(out io.Writer, label string, c *harness.Controller) error {
	_, err := fmt.Fprintf(out, "%s: %s\nModel: %s\n", label, c.SessionID(), c.Settings().Model)
	return err
}

func runCommand(ctx context.Context, prompt string, c *harness.Controller, out io.Writer) error {
	parts := strings.Fields(prompt)
	switch parts[0] {
	case "/resume":
		if len(parts) != 2 {
			return errors.New("usage: /resume <id>; use /sessions to list IDs")
		}
		if err := c.Resume(ctx, parts[1]); err != nil {
			return err
		}
		return showSession(out, "Resumed session", c)
	case "/new":
		if len(parts) != 1 {
			return errors.New("usage: /new")
		}
		if err := c.NewSession(ctx); err != nil {
			return err
		}
		return showSession(out, "New session", c)
	case "/sessions":
		if len(parts) != 1 {
			return errors.New("usage: /sessions")
		}
		sessions, err := c.Sessions(ctx)
		if err != nil {
			return fmt.Errorf("list sessions: %w", err)
		}
		if _, err := fmt.Fprintln(out, "Saved sessions (most recently updated first):"); err != nil {
			return err
		}
		for _, s := range sessions {
			marker := " "
			if s.ID == c.SessionID() {
				marker = "*"
			}
			if _, err := fmt.Fprintf(out, "%s %s  %s\n", marker, s.ID, s.UpdatedAt.Local().Format("2006-01-02 15:04:05")); err != nil {
				return err
			}
		}
		return nil
	case "/clear":
		if len(parts) != 1 {
			return errors.New("usage: /clear")
		}
		if err := c.Clear(ctx); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "Active context cleared. Saved transcript retained.")
		return err
	case "/help":
		if len(parts) != 1 {
			return errors.New("usage: /help")
		}
		_, err := fmt.Fprint(out, commandHelp)
		return err
	default:
		return fmt.Errorf("unknown command %q; type /help", parts[0])
	}
}
