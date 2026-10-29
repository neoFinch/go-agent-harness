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
	dataDir := flags.String("data-dir", ".harness", "Project-local session data directory")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected arguments; run harness -h for usage")
	}
	if llm == nil {
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
	controller, err := harness.Open(ctx, harness.Config{DataDir: path, Model: llm})
	if err != nil {
		return fmt.Errorf("start harness: %w", err)
	}
	defer controller.Close()
	if _, err := fmt.Fprintf(output, "Harness ready. Type 'exit' to quit, /clear to clear active context.\nSession: %s\nSaved in: %s\n", controller.SessionID(), filepath.Join(path, "sessions.sqlite")); err != nil {
		return err
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
		case "/clear":
			if err := controller.Clear(ctx); err != nil {
				fmt.Fprintln(errorOutput, "Clear failed:", err)
			} else {
				if _, err := fmt.Fprintln(output, "Active context cleared. Saved transcript retained."); err != nil {
					return err
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
