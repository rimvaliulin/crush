package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/agent"
	"github.com/charmbracelet/crush/internal/harness"
	"github.com/charmbracelet/crush/internal/hooks"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/session"
)

// sessionEndTimeout bounds the SessionEnd hooks so a slow script cannot
// hold the process open on exit.
const sessionEndTimeout = 2 * time.Second

// CreateSession creates a new session and fires SessionStart. Every
// session creation path goes through here so the hook is consistent.
func (app *App) CreateSession(ctx context.Context, title string) (session.Session, error) {
	return app.createSession(ctx, title, harness.DefaultName)
}

// CreateHarness creates a session that runs the named harness for its whole
// life. Unknown names are rejected so a typo cannot silently start a default
// session.
func (app *App) CreateHarness(ctx context.Context, title, harnessName string) (session.Session, error) {
	if _, ok := harness.Lookup(harnessName); !ok {
		return session.Session{}, fmt.Errorf("unknown harness %q (known: %s)", harnessName, strings.Join(harness.Names(), ", "))
	}
	return app.createSession(ctx, title, harnessName)
}

func (app *App) createSession(ctx context.Context, title, harnessName string) (session.Session, error) {
	sess, err := app.Sessions.CreateHarness(ctx, title, harnessName)
	if err != nil {
		return session.Session{}, err
	}
	app.hookRunner.Run(ctx, hooks.Event{Name: hooks.EventSessionStart, SessionID: sess.ID, Source: "startup"})
	return sess, nil
}

// resolveSession resolves which session to use for a non-interactive
// run: the given session, the most recent top-level one, or a new one.
// Resumed sessions fire SessionStart with source "resume". A SKILL.state
// session can only be created here, never resumed: the mode is fixed at
// creation.
func (app *App) resolveSession(ctx context.Context, continueSessionID string, useLast bool, harnessName string) (session.Session, error) {
	if harnessName != "" && (continueSessionID != "" || useLast) {
		return session.Session{}, fmt.Errorf("a harness only applies to a new session: a session keeps the harness it was created with")
	}
	var sess session.Session
	var err error
	switch {
	case continueSessionID != "":
		if app.Sessions.IsAgentToolSession(continueSessionID) {
			return session.Session{}, fmt.Errorf("cannot continue an agent tool session: %s", continueSessionID)
		}
		sess, err = app.Sessions.Get(ctx, continueSessionID)
		if err != nil {
			return session.Session{}, fmt.Errorf("session not found: %s", continueSessionID)
		}
		if sess.ParentSessionID != "" {
			return session.Session{}, fmt.Errorf("cannot continue a child session: %s", continueSessionID)
		}
	case useLast:
		sess, err = app.Sessions.GetLast(ctx)
		if err != nil {
			return session.Session{}, fmt.Errorf("no sessions found to continue")
		}
	case harnessName != "":
		return app.CreateHarness(ctx, agent.DefaultSessionName, harnessName)
	default:
		return app.CreateSession(ctx, agent.DefaultSessionName)
	}
	app.hookRunner.Run(ctx, hooks.Event{Name: hooks.EventSessionStart, SessionID: sess.ID, Source: "resume"})
	return sess, nil
}

// fireSessionEnd fires SessionEnd for the session the user was working
// in. It runs after agents have stopped so Interrupt precedes it.
func (app *App) fireSessionEnd() {
	ctx, cancel := context.WithTimeout(context.Background(), sessionEndTimeout)
	defer cancel()
	sessionID, _ := app.currentSessionID.Load().(string)
	app.hookRunner.Run(ctx, hooks.Event{
		Name:      hooks.EventSessionEnd,
		SessionID: sessionID,
		Reason:    "exit",
	})
}

// permissionHooks fires PermissionRequest when a prompt is shown and
// PermissionResult when it is granted, denied, or cancelled.
type permissionHooks struct {
	runner *hooks.Runner
}

func (p permissionHooks) PromptShown(ctx context.Context, req permission.PermissionRequest) {
	p.runner.Run(ctx, permissionEvent(hooks.EventPermissionRequest, req))
}

func (p permissionHooks) PromptResolved(ctx context.Context, req permission.PermissionRequest, outcome permission.Outcome) {
	ev := permissionEvent(hooks.EventPermissionResult, req)
	ev.Decision = string(outcome)
	p.runner.Run(ctx, ev)
}

func permissionEvent(name string, req permission.PermissionRequest) hooks.Event {
	input, err := json.Marshal(req.Params)
	if err != nil {
		slog.Warn("Failed to encode permission params for hook", "tool", req.ToolName, "error", err)
		input = []byte("{}")
	}
	return hooks.Event{
		Name:      name,
		SessionID: req.SessionID,
		Tool:      &hooks.ToolCall{ID: req.ToolCallID, Name: req.ToolName, Input: string(input)},
	}
}
