// Package harness defines how a session runs. A harness owns everything a
// session does differently from the default transcript-replaying agent: the
// specification appended to the system prompt, the per-session state, the
// shape of the step prompt, the tools the model sees and the status reported
// to crush_info.
//
// A session names its harness once, at creation, and keeps it until it is
// deleted. Adding a harness means writing an implementation, appending its
// Info to Catalog and registering it in app.New; the session, proto, backend
// and agent layers stay untouched.
package harness

import (
	"context"

	"charm.land/fantasy"
)

// DefaultName is the name of the harness every session runs unless it picks
// another one. It is spelled out here rather than left empty so the stored
// session names a harness like any other.
const DefaultName = "default"

// SkillState is the name of the bounded-prompt harness.
const SkillState = "skillstate"

// Info describes a harness before a session exists: the CLI validates
// --harness against the catalog, the new-session dialog builds its entries
// from it.
type Info struct {
	Name  string
	Label string
}

// Catalog lists the harnesses compiled into this binary. A harness without an
// entry here cannot be selected, even if it is registered.
var Catalog = []Info{
	{Name: DefaultName, Label: "Default"},
	{Name: SkillState, Label: "SKILL.state"},
}

// Lookup returns the catalog entry for a harness name.
func Lookup(name string) (Info, bool) {
	for _, info := range Catalog {
		if info.Name == name {
			return info, true
		}
	}
	return Info{}, false
}

// Names returns every selectable harness name.
func Names() []string {
	names := make([]string, 0, len(Catalog))
	for _, info := range Catalog {
		names = append(names, info.Name)
	}
	return names
}

// Harness shapes one session's execution.
type Harness interface {
	// Name is the catalog name stored on the session.
	Name() string

	// SystemSpec is the text appended to the system prompt. It goes last:
	// the tail of a long prompt is what the model reads most reliably.
	SystemSpec() string

	// Prepare sets up whatever the session needs before a turn starts. It
	// runs on every turn; an implementation must be idempotent.
	Prepare(ctx context.Context, sessionID, goal string) error

	// Shape returns the messages of one step. It must never fail: when the
	// per-session data is unavailable the result stays bounded and carries
	// the reason, so a failure cannot silently restore the full transcript.
	Shape(ctx context.Context, sessionID string, messages []fantasy.Message) []fantasy.Message

	// Tools are the tools that belong to this harness. They are hidden
	// from sessions that run a different harness.
	Tools() []fantasy.AgentTool

	// Status is the harness section of crush_info. An error is reported to
	// the model as "unavailable" and never hides the rest of the report.
	Status(ctx context.Context, sessionID string) (string, error)
}

// Registry maps session harness names to live implementations. An empty name
// is read as DefaultName so sessions created before the default harness existed
// keep running it.
type Registry map[string]Harness

// New builds a registry from implementations.
func New(harnesses ...Harness) Registry {
	registry := make(Registry, len(harnesses))
	for _, h := range harnesses {
		if h == nil {
			continue
		}
		registry[h.Name()] = h
	}
	return registry
}

// Get returns the harness for a session name. An empty name means the default
// harness; nil comes back only when nothing is registered for the name.
func (r Registry) Get(name string) Harness {
	if name == "" {
		name = DefaultName
	}
	h, ok := r[name]
	if !ok {
		return nil
	}
	return h
}

// Add returns the available tools plus the tools of every registered harness.
// The agent serves every session, so harness tools are registered once and
// filtered per session.
func (r Registry) Add(available []fantasy.AgentTool) []fantasy.AgentTool {
	for _, h := range r {
		available = append(available, h.Tools()...)
	}
	return available
}

// FilterTools drops the tools owned by harnesses other than active, so a
// session only sees what its own harness provides. A nameless session is read
// the way Get reads it.
func (r Registry) FilterTools(active string, available []fantasy.AgentTool) []fantasy.AgentTool {
	if h := r.Get(active); h != nil {
		active = h.Name()
	}
	hidden := make(map[string]struct{})
	for name, h := range r {
		if name == active {
			continue
		}
		for _, tool := range h.Tools() {
			hidden[tool.Info().Name] = struct{}{}
		}
	}
	if len(hidden) == 0 {
		return available
	}
	filtered := make([]fantasy.AgentTool, 0, len(available))
	for _, tool := range available {
		if _, off := hidden[tool.Info().Name]; off {
			continue
		}
		filtered = append(filtered, tool)
	}
	return filtered
}
