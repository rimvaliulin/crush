// Package skillstate owns the explicit execution state of a SKILL.state
// session: one row per session in the Crush database.
//
// The state is a small JSON object. The runtime, not the model, is the
// authority on its shape: patches are merged and validated here before the
// row is replaced inside a transaction, so a malformed update can never
// corrupt the state the next step reads. Because the row belongs to a
// session, a new session starts a fresh goal, and deleting the session
// deletes its state.
package skillstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/db"
)

// DefaultMaxBytes bounds the state when the caller does not set a cap.
const DefaultMaxBytes = 4096

// Statuses are the accepted values of the "status" key.
var Statuses = []string{"active", "done"}

// ErrNoState reports a session with no execution state, so the caller can
// tell it apart from a database failure.
var ErrNoState = errors.New("no execution state for this session")

// kind is the JSON shape a key must carry.
type kind string

const (
	kindString kind = "string"
	kindList   kind = "list"
	kindObject kind = "object"
)

type field struct {
	key  string
	kind kind
}

var (
	required = []field{
		{"status", kindString},
		{"goal", kindString},
		{"constraints", kindList},
		{"verified", kindObject},
		{"current", kindString},
		{"next", kindList},
		{"facts", kindObject},
		{"errors", kindObject},
	}
	optional = []field{
		{"decisions", kindObject},
	}
	listOfStrings = []string{"constraints", "next"}
)

// Store reads and writes execution states. It is safe for concurrent use:
// every write runs in its own transaction and SQLite serialises writers.
type Store struct {
	conn     *sql.DB
	maxBytes int
}

// New returns a store backed by the given connection. maxBytes caps the
// compact state carried by every step; zero or a negative value restores
// DefaultMaxBytes.
func New(conn *sql.DB, maxBytes int) *Store {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	return &Store{conn: conn, maxBytes: maxBytes}
}

// MaxStateBytes returns the cap this store enforces on the compact state.
func (s *Store) MaxStateBytes() int {
	if s.maxBytes <= 0 {
		return DefaultMaxBytes
	}
	return s.maxBytes
}

// Get returns the state of a session.
func (s *Store) Get(ctx context.Context, sessionID string) (map[string]any, error) {
	return readState(ctx, db.New(s.conn), sessionID)
}

// Status returns the status of a session's state and when it was last
// written.
func (s *Store) Status(ctx context.Context, sessionID string) (string, time.Time, error) {
	row, err := db.New(s.conn).GetSkillState(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, ErrNoState
	}
	if err != nil {
		return "", time.Time{}, err
	}
	state, err := Parse([]byte(row.State))
	if err != nil {
		return "", time.Time{}, err
	}
	return statusOf(state), time.Unix(row.UpdatedAt, 0), nil
}

// Ensure returns the execution state of a session, creating it from the
// prompt when the session has none. A state whose goal is done is replaced:
// work after a finished goal is a new goal.
func (s *Store) Ensure(ctx context.Context, sessionID, prompt string) (map[string]any, error) {
	if sessionID == "" {
		return nil, ErrNoState
	}
	seed := newState(prompt)
	raw, err := json.Marshal(seed)
	if err != nil {
		return nil, err
	}
	// The upsert inserts a missing row and only replaces a finished goal,
	// returning what it wrote. An active state matches nothing and is read
	// below, so the common case is a single statement.
	queries := db.New(s.conn)
	written, err := queries.EnsureSkillState(ctx, db.EnsureSkillStateParams{
		SessionID: sessionID,
		State:     string(raw),
	})
	switch {
	case err == nil:
		return Parse([]byte(written))
	case !errors.Is(err, sql.ErrNoRows):
		return nil, err
	}
	return readState(ctx, queries, sessionID)
}

// Patch merges a patch into the state, validates the result and stores it.
// On any error the row is left untouched.
func (s *Store) Patch(ctx context.Context, sessionID string, patch map[string]any) (int, error) {
	if sessionID == "" {
		return 0, ErrNoState
	}
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	queries := db.New(tx)
	current, err := readState(ctx, queries, sessionID)
	if err != nil {
		return 0, err
	}

	next := clone(current)
	Merge(next, patch)

	if problems := Validate(next, current); len(problems) > 0 {
		return 0, errors.New(strings.Join(problems, "; "))
	}

	// The cap is checked on the compact form: that is what every step
	// receives in its prompt, and the stored row is only for the runtime.
	size := len(Compact(next))
	if cap := s.MaxStateBytes(); size > cap {
		return 0, fmt.Errorf(
			"state would be %d bytes (cap %d); prune stale entries",
			size, cap,
		)
	}

	raw, err := json.Marshal(next)
	if err != nil {
		return 0, err
	}
	if err := queries.UpsertSkillState(ctx, db.UpsertSkillStateParams{
		SessionID: sessionID,
		State:     string(raw),
	}); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return size, nil
}

// deleteState removes the state of a session. The row would also go away
// with the session itself; this exists for tests and for an explicit reset.
func (s *Store) deleteState(ctx context.Context, sessionID string) error {
	return db.New(s.conn).DeleteSkillState(ctx, sessionID)
}

func readState(ctx context.Context, queries *db.Queries, sessionID string) (map[string]any, error) {
	row, err := queries.GetSkillState(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoState
	}
	if err != nil {
		return nil, err
	}
	return Parse([]byte(row.State))
}

// newState seeds a state for a prompt.
func newState(prompt string) map[string]any {
	return map[string]any{
		"status":      "active",
		"goal":        goalLine(prompt),
		"constraints": []any{},
		"verified":    map[string]any{},
		"current":     "",
		"next":        []any{},
		"facts":       map[string]any{},
		"errors":      map[string]any{},
	}
}

// statusOf returns the status of a state, or "" when it is unreadable.
func statusOf(state map[string]any) string {
	value, _ := state["status"].(string)
	return value
}

// Parse decodes a state document.
func Parse(raw []byte) (map[string]any, error) {
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("not valid JSON: %w", err)
	}
	if state == nil {
		return nil, errors.New("root must be a JSON object")
	}
	return state, nil
}

// goalLine turns a prompt into a one-line goal, since the state keeps a
// single line and the tool cannot change it afterwards.
func goalLine(prompt string) string {
	line := strings.TrimSpace(prompt)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	const limit = 200
	if runes := []rune(line); len(runes) > limit {
		line = string(runes[:limit])
	}
	if line == "" {
		return "unspecified"
	}
	return line
}

// Compact renders the state as one JSON line for prompt injection.
func Compact(state map[string]any) string {
	raw, err := json.Marshal(state)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// Merge applies a patch in place: objects merge key by key, a null value
// deletes the key, and every other value replaces what was there.
func Merge(target, patch map[string]any) {
	for key, value := range patch {
		if value == nil {
			delete(target, key)
			continue
		}
		patchObject, patchIsObject := value.(map[string]any)
		targetObject, targetIsObject := target[key].(map[string]any)
		if patchIsObject && targetIsObject {
			Merge(targetObject, patchObject)
			continue
		}
		target[key] = value
	}
}

// Validate checks the state schema. When original is non-nil the goal must
// stay unchanged.
func Validate(state, original map[string]any) []string {
	var problems []string

	for _, f := range required {
		value, ok := state[f.key]
		switch {
		case !ok:
			problems = append(problems, "missing required key: "+f.key)
		case !matches(value, f.kind):
			problems = append(problems, fmt.Sprintf("key '%s' must be %s", f.key, f.kind))
		}
	}
	for _, f := range optional {
		if value, ok := state[f.key]; ok && !matches(value, f.kind) {
			problems = append(problems, fmt.Sprintf("key '%s' must be %s", f.key, f.kind))
		}
	}

	for _, key := range listOfStrings {
		list, ok := state[key].([]any)
		if !ok {
			continue
		}
		for _, item := range list {
			if _, isString := item.(string); !isString {
				problems = append(problems, fmt.Sprintf("'%s' must be a list of strings", key))
				break
			}
		}
	}

	if status, ok := state["status"].(string); ok && !slices.Contains(Statuses, status) {
		problems = append(problems, "status must be one of: "+strings.Join(Statuses, ", "))
	}
	if goal, ok := state["goal"].(string); ok && strings.TrimSpace(goal) == "" {
		problems = append(problems, "goal must not be empty")
	}
	if original != nil {
		next, _ := state["goal"].(string)
		previous, _ := original["goal"].(string)
		if next != previous {
			problems = append(problems, "goal is immutable")
		}
	}

	return problems
}

func matches(value any, want kind) bool {
	switch want {
	case kindString:
		_, ok := value.(string)
		return ok
	case kindList:
		_, ok := value.([]any)
		return ok
	case kindObject:
		_, ok := value.(map[string]any)
		return ok
	}
	return false
}

func clone(state map[string]any) map[string]any {
	raw, err := json.Marshal(state)
	if err != nil {
		return map[string]any{}
	}
	var copy map[string]any
	if err := json.Unmarshal(raw, &copy); err != nil {
		return map[string]any{}
	}
	return copy
}
