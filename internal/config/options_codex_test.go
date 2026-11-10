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
