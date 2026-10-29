// PROTOTYPE: request-budget policy hooks, not a production tokenizer.
package adkprototype

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

type BudgetTargets struct{ Output, Input, Target, Recent int }

// Count must account for the assembled request, not just the latest prompt.
// Compact is injected so this probe does not pretend to implement a real summarizer.
type BudgetGate struct {
	ContextBudget  int
	OutputPercent  int
	ModelWindow    int
	ModelMaxOutput int
	Count          func(*model.LLMRequest) (int, error)
	Compact        func(context.Context, *model.LLMRequest, BudgetTargets) error
}

func (g *BudgetGate) targets() (BudgetTargets, error) {
	percent := g.OutputPercent
	if percent == 0 {
		percent = 25
	}
	if percent < 1 || percent > 99 || g.ContextBudget < 2 || g.ContextBudget > 2147483647 || g.ModelMaxOutput < 1 || g.ModelWindow < g.ContextBudget {
		return BudgetTargets{}, errors.New("invalid or model-incompatible context budget")
	}
	reserve := int((int64(g.ContextBudget)*int64(percent) + 99) / 100)
	if reserve > g.ModelMaxOutput {
		reserve = g.ModelMaxOutput
	}
	input := g.ContextBudget - reserve
	if input < 1 {
		return BudgetTargets{}, errors.New("no input capacity remains")
	}
	return BudgetTargets{Output: reserve, Input: input, Target: input * 60 / 100, Recent: input * 40 / 100}, nil
}

func cloneBudgetRequest(req *model.LLMRequest) (*model.LLMRequest, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	var copy model.LLMRequest
	if err := json.Unmarshal(data, &copy); err != nil {
		return nil, err
	}
	// Tool implementations are deliberately excluded from JSON; preserve handles.
	copy.Tools = req.Tools
	return &copy, nil
}

func (g *BudgetGate) BeforeModel(ctx agent.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
	targets, err := g.targets()
	if err != nil {
		return nil, err
	}
	if g.Count == nil {
		return nil, errors.New("request token counter is required")
	}
	original, err := cloneBudgetRequest(req)
	if err != nil {
		return nil, err
	}
	if original.Config == nil {
		original.Config = &genai.GenerateContentConfig{}
	}
	original.Config.MaxOutputTokens = int32(targets.Output)
	count := func(candidate *model.LLMRequest) (int, error) {
		used, err := g.Count(candidate)
		if err != nil {
			return 0, fmt.Errorf("count request: %w", err)
		}
		if used < 0 {
			return 0, errors.New("negative request token count")
		}
		return used, nil
	}
	used, err := count(original)
	if err != nil {
		return nil, err
	}
	if int64(used)*10 < int64(targets.Input)*9 {
		*req = *original
		return nil, nil
	}
	if g.Compact == nil {
		return nil, errors.New("compaction required before model call")
	}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		candidate, err := cloneBudgetRequest(original)
		if err != nil {
			return nil, err
		}
		if err := g.Compact(ctx, candidate, targets); err != nil {
			last = err
			continue
		}
		// The compactor only controls history. It must not change instructions,
		// tool declarations or output reserve while shortening the request.
		candidate.Config = original.Config
		used, err := count(candidate)
		if err != nil {
			return nil, err
		}
		if used > targets.Target {
			last = fmt.Errorf("compacted input %d exceeds prototype target %d", used, targets.Target)
			continue
		}
		*req = *candidate
		return nil, nil
	}
	return nil, fmt.Errorf("compaction failed after 3 attempts; model request blocked: %w", last)
}
