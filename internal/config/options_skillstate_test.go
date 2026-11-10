package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOptionsGetSkillStateMaxBytes(t *testing.T) {
	t.Parallel()

	skillState := func(v int) *HarnessOptions {
		return &HarnessOptions{SkillState: &SkillStateOptions{MaxStateBytes: ptr(v)}}
	}

	tests := []struct {
		name     string
		options  *Options
		expected int
	}{
		{"nil options", nil, DefaultSkillStateMaxBytes},
		{"unset", &Options{}, DefaultSkillStateMaxBytes},
		{"harness without skillstate", &Options{Harness: &HarnessOptions{}}, DefaultSkillStateMaxBytes},
		{"zero means default", &Options{Harness: skillState(0)}, DefaultSkillStateMaxBytes},
		{"negative means default", &Options{Harness: skillState(-5)}, DefaultSkillStateMaxBytes},
		{"configured", &Options{Harness: skillState(8192)}, 8192},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, tt.options.GetSkillStateMaxBytes())
		})
	}
}

func TestOptionsSkillStateMaxBytesFromJSON(t *testing.T) {
	t.Parallel()

	var cfg Config
	require.NoError(t, json.Unmarshal([]byte(`{"options":{"harness":{"skillstate":{"max_state_bytes":2048}}}}`), &cfg))
	require.Equal(t, 2048, cfg.Options.GetSkillStateMaxBytes())
}
