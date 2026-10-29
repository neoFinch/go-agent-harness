package agent

import "context"

type Message struct {
	Role    string
	Content string
}

type Model interface {
	Generate(ctx context.Context, messages []Message, onText func(string)) error
}
