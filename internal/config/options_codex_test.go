package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOptionsGetCodexMaxOutputBytes(t *testing.T) {
	t.Parallel()

	codex := func(v int) *HarnessOptions {
		return &HarnessOptions{Codex: &CodexOptions{MaxOutputBytes: ptr(v)}}
	}

	tests := []struct {
		name     string
		options  *Options
		expected int
	}{
		{"nil options", nil, DefaultCodexMaxOutputBytes},
		{"unset", &Options{}, DefaultCodexMaxOutputBytes},
		{"harness without codex", &Options{Harness: &HarnessOptions{}}, DefaultCodexMaxOutputBytes},
		{"zero means default", &Options{Harness: codex(0)}, DefaultCodexMaxOutputBytes},
		{"negative means default", &Options{Harness: codex(-1)}, DefaultCodexMaxOutputBytes},
		{"configured", &Options{Harness: codex(24000)}, 24000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, tt.options.GetCodexMaxOutputBytes())
		})
	}
}

func TestOptionsCodexMaxOutputBytesFromJSON(t *testing.T) {
	t.Parallel()

	var cfg Config
	require.NoError(t, json.Unmarshal([]byte(`{"options":{"harness":{"codex":{"max_output_bytes":24000}}}}`), &cfg))
	require.Equal(t, 24000, cfg.Options.GetCodexMaxOutputBytes())
}

func TestOptionsGetCodexInstructions(t *testing.T) {
	t.Parallel()

	instructions := func(v bool) *HarnessOptions {
		return &HarnessOptions{Codex: &CodexOptions{Instructions: ptr(v)}}
	}

	tests := []struct {
		name     string
		options  *Options
		expected bool
	}{
		{"nil options", nil, true},
		{"unset", &Options{}, true},
		{"harness without codex", &Options{Harness: &HarnessOptions{}}, true},
		{"explicitly on", &Options{Harness: instructions(true)}, true},
		{"explicitly off", &Options{Harness: instructions(false)}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, tt.options.GetCodexInstructions())
		})
	}
}

func TestOptionsCodexInstructionsFromJSON(t *testing.T) {
	t.Parallel()

	var withCodex Config
	require.NoError(t, json.Unmarshal([]byte(`{"options":{"harness":{"codex":{"instructions":false}}}}`), &withCodex))
	require.False(t, withCodex.Options.GetCodexInstructions())

	var plain Config
	require.NoError(t, json.Unmarshal([]byte(`{}`), &plain))
	require.True(t, plain.Options.GetCodexInstructions())
}
