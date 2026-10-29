// Package harness coordinates the CLI's session and transcript policy around ADK.
package harness

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/platform"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/session/database"
	"google.golang.org/genai"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const appName = "agent-harness"
const userID = "local"

type Config struct {
	DataDir string
	// Model supplies the startup/new-session default, independently of resumed settings.
	Model     model.LLM
	SessionID string
	// ModelFactory restores saved model choices that differ from Model.Name().
	ModelFactory func(context.Context, string) (model.LLM, error)
}

// Turn supplements ADK's structured events with visible output and completion
// status. Only completed invocations are eligible for subsequent model history.
type Turn struct {
	Sequence     uint64 `gorm:"primaryKey;autoIncrement"`
	ID           string `gorm:"uniqueIndex;not null"`
	SessionID    string `gorm:"index;not null"`
	InvocationID string
	Kind         string
	Prompt       string
	Text         string
	Status       string
	Error        string
}

func (Turn) TableName() string { return "harness_turns" }

type Controller struct {
	mu           sync.Mutex
	db           *gorm.DB
	connection   *sql.DB
	runtime      *runner.Runner
	service      session.Service
	sessionID    string
	active       *Turn
	defaults     model.LLM
	modelFactory func(context.Context, string) (model.LLM, error)
	state        savedState
}

// Open opens local storage and starts a new ADK session, or resumes SessionID.
// The caller owns the controller and must Close it when finished.
func Open(ctx context.Context, cfg Config) (*Controller, error) {
	if cfg.Model == nil {
		return nil, errors.New("model is required")
	}
	if strings.TrimSpace(cfg.Model.Name()) == "" {
		return nil, errors.New("model name is required")
	}
	if cfg.DataDir == "" {
		return nil, errors.New("data directory is required")
	}
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	path := filepath.Join(cfg.DataDir, "sessions.sqlite")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err == nil {
		if err := file.Close(); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open session database: %w", err)
	}
	connection, err := db.DB()
	if err != nil {
		return nil, err
	}
	connection.SetMaxOpenConns(1)
	ok := false
	defer func() {
		if !ok {
			connection.Close()
		}
	}()
	for _, pragma := range []string{"PRAGMA busy_timeout=5000", "PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL", "PRAGMA foreign_keys=ON"} {
		if err := db.WithContext(ctx).Exec(pragma).Error; err != nil {
			return nil, err
		}
	}
	svc, err := database.NewSessionServiceFromDB(db)
	if err != nil {
		return nil, err
	}
	if err := database.AutoMigrate(svc); err != nil {
		return nil, err
	}
	if err := db.WithContext(ctx).AutoMigrate(&Turn{}); err != nil {
		return nil, err
	}
	c := &Controller{db: db, connection: connection, service: svc, defaults: cfg.Model, modelFactory: cfg.ModelFactory}
	if cfg.SessionID == "" {
		err = c.NewSession(ctx)
	} else {
		err = c.Resume(ctx, cfg.SessionID)
	}
	if err != nil {
		return nil, err
	}
	ok = true
	return c, nil
}

func (c *Controller) SessionID() string { return c.sessionID }
func (c *Controller) Close() error      { return c.connection.Close() }

// Transcript reads captured turns from storage, including earlier sessions whose
// IDs the user knows. It is not a model-accessible cross-session history tool.
func (c *Controller) Transcript(ctx context.Context, sessionID string) ([]Turn, error) {
	var turns []Turn
	err := c.db.WithContext(ctx).Where("session_id = ? AND kind = ?", sessionID, "turn").Order("sequence").Find(&turns).Error
	return turns, err
}

// Clear resets active context without deleting saved records or changing session
// identity. A durable marker prevents old events reappearing in model history.
func (c *Controller) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	next := c.state
	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		marker := &Turn{ID: rand.Text(), SessionID: c.sessionID, Kind: "clear", Status: "completed"}
		if err := tx.Create(marker).Error; err != nil {
			return err
		}
		next.ClearedThrough = marker.Sequence
		svc, err := database.NewSessionServiceFromDB(tx)
		if err != nil {
			return err
		}
		return writeState(ctx, svc, c.sessionID, next)
	})
	if err == nil {
		c.state = next
	}
	return err
}

// activeHistory uses ADK events as the history source, with our success/clear
// policy applied as a view. It does not reconstruct provider-specific messages.
func (c *Controller) activeHistory(ctx agent.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
	if c.active == nil {
		return nil, errors.New("no active turn")
	}
	invocation := ctx.InvocationID()
	if c.active.InvocationID == "" {
		if err := c.db.WithContext(ctx).Model(&Turn{}).Where("id = ?", c.active.ID).Update("invocation_id", invocation).Error; err != nil {
			return nil, err
		}
		c.active.InvocationID = invocation
	}
	var completed []string
	if err := c.db.WithContext(ctx).Model(&Turn{}).Where("session_id = ? AND kind = ? AND status = ? AND sequence > ?", c.sessionID, "turn", "completed", c.state.ClearedThrough).Pluck("invocation_id", &completed).Error; err != nil {
		return nil, err
	}
	allowed := map[string]bool{invocation: true}
	for _, id := range completed {
		allowed[id] = true
	}
	var contents []*genai.Content
	stored, err := c.service.Get(ctx, &session.GetRequest{AppName: appName, UserID: userID, SessionID: c.sessionID})
	if err != nil {
		return nil, err
	}
	for event := range stored.Session.Events().All() {
		if allowed[event.InvocationID] && event.Content != nil {
			contents = append(contents, event.Content)
		}
	}
	req.Contents = contents
	return nil, nil
}

func (c *Controller) Run(ctx context.Context, prompt string, onText func(string)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if strings.TrimSpace(prompt) == "" {
		return errors.New("prompt cannot be empty")
	}
	turn := &Turn{ID: rand.Text(), SessionID: c.sessionID, Kind: "turn", Prompt: prompt, Status: "incomplete"}
	if err := c.db.WithContext(ctx).Create(turn).Error; err != nil {
		return fmt.Errorf("save input: %w", err)
	}
	c.active = turn
	defer func() { c.active = nil }()
	finish := func(cause error) error {
		status, message := "completed", ""
		if cause != nil {
			status, message = "failed", cause.Error()
			if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
				status = "cancelled"
			}
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		err := c.db.WithContext(cleanup).Model(&Turn{}).Where("id = ?", turn.ID).Updates(map[string]any{"status": status, "error": message}).Error
		if err != nil {
			err = fmt.Errorf("save turn status: %w", err)
		}
		return errors.Join(cause, err)
	}
	emit := func(value string) error {
		if value == "" {
			return nil
		}
		result := c.db.WithContext(ctx).Model(&Turn{}).Where("id = ? AND status = ?", turn.ID, "incomplete").UpdateColumn("text", gorm.Expr("text || ?", value))
		if result.Error != nil {
			return fmt.Errorf("save output: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return errors.New("save output: active turn missing")
		}
		if onText != nil {
			onText(value)
		}
		return nil
	}
	ctx = platform.WithTaskRunner(ctx, func(ctx context.Context, tasks []func(context.Context)) {
		for _, task := range tasks {
			task(ctx)
		}
	})
	partialForResponse, finalSeen := false, false
	for event, err := range c.runtime.Run(ctx, userID, c.sessionID, genai.NewContentFromText(prompt, genai.RoleUser), agent.RunConfig{StreamingMode: agent.StreamingModeSSE}) {
		if err != nil {
			return finish(err)
		}
		if err := ctx.Err(); err != nil {
			return finish(err)
		}
		if event == nil {
			return finish(errors.New("ADK returned an empty event"))
		}
		visiblePartial := false
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
				if event.Partial {
					visiblePartial = true
				}
			}
		}
		if event.Partial {
			partialForResponse = partialForResponse || visiblePartial
		} else {
			partialForResponse = false
		}
		if event.ErrorCode != "" {
			return finish(fmt.Errorf("model error %s: %s", event.ErrorCode, event.ErrorMessage))
		}
		if event.Interrupted {
			return finish(errors.New("model response interrupted"))
		}
		if event.FinishReason != "" && event.FinishReason != genai.FinishReasonUnspecified && event.FinishReason != genai.FinishReasonStop {
			return finish(fmt.Errorf("model response incomplete: %s", event.FinishReason))
		}
		if event.IsFinalResponse() && event.Content != nil {
			finalSeen = true
		}
		if err := ctx.Err(); err != nil {
			return finish(err)
		}
	}
	if !finalSeen {
		return finish(errors.New("stream ended without a final response"))
	}
	return finish(nil)
}
