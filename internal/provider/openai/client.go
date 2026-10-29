package openai

import (
	"context"
	"fmt"

	"github.com/neoFinch/agent-harness/internal/agent"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

// Client implements text generation using OpenAI's Responses API.
type Client struct {
	sdk   openaisdk.Client
	model string
}

// Verify at compile time that Client implements the harness's Model interface.
var _ agent.Model = (*Client)(nil)

// New creates a reusable client. The SDK reads OPENAI_API_KEY from the environment.
func New(model string) *Client {
	return &Client{
		sdk:   openaisdk.NewClient(),
		model: model,
	}
}

func (c *Client) Generate(
	ctx context.Context,
	messages []agent.Message,
	onText func(string),
) error {
	input := make([]responses.ResponseInputItemUnionParam, 0, len(messages))

	for _, message := range messages {
		input = append(input, responses.ResponseInputItemParamOfMessage(
			message.Content,
			responses.EasyInputMessageRole(message.Role),
		))
	}
	stream := c.sdk.Responses.NewStreaming(ctx, responses.ResponseNewParams{
		Model: c.model,
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		},
	})
	defer stream.Close()

	completed := false

	for stream.Next() {
		event := stream.Current()

		switch event.Type {
		case "response.output_text.delta":
			onText(event.Delta)

		case "response.completed":
			completed = true

		case "response.failed", "response.incomplete":
			return fmt.Errorf("OpenAI generation ended: %s", event.Type)

		case "error":
			return fmt.Errorf("OpenAI streaming error")
		}
	}

	if err := stream.Err(); err != nil {
		return fmt.Errorf("stream OpenAI response: %w", err)
	}
	if !completed {
		return fmt.Errorf("OpenAI stream ended before completion")
	}

	return nil
}
