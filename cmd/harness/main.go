package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/neoFinch/agent-harness/internal/agent"
	"github.com/neoFinch/agent-harness/internal/provider/openai"
)

func main() {
	fmt.Println("Harness ready. Type 'exit' to quit.")
	model := openai.New("gpt-4o-mini")
	ctx := context.Background()
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("You: ")

	var history []agent.Message

	for scanner.Scan() {
		prompt := scanner.Text()
		if strings.TrimSpace(prompt) == "exit" {
			break
		}
		if strings.TrimSpace(prompt) == "/clear" {
			history = nil
			fmt.Println("Conversation cleared.\n\n YOu: ")
			continue
		}

		if strings.TrimSpace(prompt) == "" {
			fmt.Print("You: ")
			continue
		}

		messages := append(history, agent.Message{
			Role:    "user",
			Content: prompt,
		})
		var answer strings.Builder

		fmt.Print("Assistant: ")
		err := runPrompt(ctx, model, messages, func(chunk string) {
			fmt.Print(chunk)
			answer.WriteString(chunk)
		})
		fmt.Println()

		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		} else {
			// Commit the exchange only after the response completes successfully.
			history = append(messages, agent.Message{
				Role:    "assistant",
				Content: answer.String(),
			})
		}

		fmt.Print("\nYou: ")
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "Reading prompt:", err)
		os.Exit(1)
	}
}

func runPrompt(ctx context.Context, model agent.Model, messages []agent.Message, onText func(string)) error {
	return model.Generate(ctx, messages, onText)
}
