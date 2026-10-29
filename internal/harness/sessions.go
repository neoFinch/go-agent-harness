package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/plugin"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
)

type SessionInfo struct {
	ID        string
	UpdatedAt time.Time
}

// Sessions discovers saved conversations in this data directory.
func (c *Controller) Sessions(ctx context.Context) ([]SessionInfo, error) {
	listed, err := c.service.List(ctx, &session.ListRequest{AppName: appName, UserID: userID})
	if err != nil {
		return nil, err
	}
	result := make([]SessionInfo, 0, len(listed.Sessions))
	for _, s := range listed.Sessions {
		result = append(result, SessionInfo{ID: s.ID(), UpdatedAt: s.LastUpdateTime()})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result, nil
}

// Settings contains the settings implemented so far. Budget settings are added
// with budget enforcement, rather than presenting an allowance we cannot enforce.
type Settings struct {
	Model string `json:"model"`
}

// StateKey is the versioned harness checkpoint/settings envelope in ADK state.
// It is session-scoped; no app: or user: prefix may be used here.
const StateKey = "harness.session"

type savedState struct {
	Version        int      `json:"version"`
	Settings       Settings `json:"settings"`
	ClearedThrough uint64   `json:"cleared_through"`
}

func (c *Controller) Settings() Settings {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state.Settings
}

func encodeState(state savedState) string { data, _ := json.Marshal(state); return string(data) }

func writeState(ctx context.Context, svc session.Service, id string, state savedState) error {
	stored, err := svc.Get(ctx, &session.GetRequest{AppName: appName, UserID: userID, SessionID: id})
	if err != nil {
		return err
	}
	event := session.NewEvent(ctx, "")
	event.Author = "harness"
	event.Actions.StateDelta[StateKey] = encodeState(state)
	return svc.AppendEvent(ctx, stored.Session, event)
}

func (c *Controller) readState(ctx context.Context, s session.Session) (savedState, bool, error) {
	value, err := s.State().Get(StateKey)
	if errors.Is(err, session.ErrStateKeyNotExist) {
		// S01 sessions predate explicit checkpoint/settings state. Preserve their
		// clear boundary and adopt the caller's current model exactly once.
		state := savedState{Version: 1, Settings: Settings{Model: c.defaults.Name()}}
		err := c.db.WithContext(ctx).Model(&Turn{}).Where("session_id = ? AND kind = ?", s.ID(), "clear").Select("COALESCE(MAX(sequence), 0)").Scan(&state.ClearedThrough).Error
		return state, true, err
	}
	if err != nil {
		return savedState{}, false, err
	}
	raw, ok := value.(string)
	var state savedState
	if !ok {
		return state, false, errors.New("invalid saved context checkpoint: expected JSON string")
	}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return state, false, fmt.Errorf("invalid saved context checkpoint: %w", err)
	}
	if state.Version != 1 || strings.TrimSpace(state.Settings.Model) == "" {
		return state, false, errors.New("invalid or unsupported saved context checkpoint/settings; session was not changed")
	}
	if state.ClearedThrough != 0 {
		var count int64
		if err := c.db.WithContext(ctx).Model(&Turn{}).Where("session_id = ? AND sequence = ? AND kind = ?", s.ID(), state.ClearedThrough, "clear").Count(&count).Error; err != nil {
			return state, false, err
		}
		if count != 1 {
			return state, false, errors.New("invalid saved context checkpoint: clear reference is missing; session was not changed")
		}
	}
	return state, false, nil
}

func (c *Controller) buildRuntime(ctx context.Context, name string) (*runner.Runner, error) {
	llm := c.defaults
	if name != llm.Name() {
		if c.modelFactory == nil {
			return nil, fmt.Errorf("saved model %q is unavailable; configure a model factory to resume", name)
		}
		var err error
		llm, err = c.modelFactory(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("restore model %q: %w", name, err)
		}
		if llm == nil || llm.Name() != name {
			return nil, fmt.Errorf("model factory did not provide saved model %q", name)
		}
	}
	a, err := llmagent.New(llmagent.Config{Name: "harness", Model: llm, BeforeModelCallbacks: []llmagent.BeforeModelCallback{c.activeHistory}})
	if err != nil {
		return nil, err
	}
	// ADK v2.6.0 mutates Actions.Compaction in Runner while the producer can
	// still read the original. Give Runner its own EventActions value.
	isolate, err := plugin.New(plugin.Config{Name: "isolate-runner-events", OnEventCallback: func(_ agent.InvocationContext, event *session.Event) (*session.Event, error) {
		copied := *event
		return &copied, nil
	}})
	if err != nil {
		return nil, err
	}
	return runner.New(runner.Config{AppName: appName, Agent: a, SessionService: c.service, PluginConfig: runner.PluginConfig{Plugins: []*plugin.Plugin{isolate}}})
}

// NewSession starts a distinct conversation, keeping every earlier transcript.
func (c *Controller) NewSession(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	runtime, err := c.buildRuntime(ctx, c.defaults.Name())
	if err != nil {
		return err
	}
	state := savedState{Version: 1, Settings: Settings{Model: c.defaults.Name()}}
	created, err := c.service.Create(ctx, &session.CreateRequest{AppName: appName, UserID: userID, State: map[string]any{StateKey: encodeState(state)}})
	if err != nil {
		return err
	}
	c.sessionID, c.state, c.runtime = created.Session.ID(), state, runtime
	return nil
}

// Resume validates a saved conversation before replacing the current session.
// It never calls the model or creates a replacement conversation on failure.
func (c *Controller) Resume(ctx context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if strings.TrimSpace(id) == "" {
		return errors.New("resume requires a session ID; use /sessions to list saved sessions")
	}
	stored, err := c.service.Get(ctx, &session.GetRequest{AppName: appName, UserID: userID, SessionID: id})
	if err != nil {
		return fmt.Errorf("resume session %q (use /sessions to list IDs): %w", id, err)
	}
	state, legacy, err := c.readState(ctx, stored.Session)
	if err != nil {
		return fmt.Errorf("resume session %q: %w", id, err)
	}
	runtime, err := c.buildRuntime(ctx, state.Settings.Model)
	if err != nil {
		return err
	}
	if legacy {
		if err := writeState(ctx, c.service, id, state); err != nil {
			return fmt.Errorf("save legacy session checkpoint: %w", err)
		}
	}
	c.sessionID, c.state, c.runtime = id, state, runtime
	return nil
}
