package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureDir_Creates(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b", "c")
	result, err := EnsureDir(dir)
	require.NoError(t, err)
	assert.Equal(t, dir, result)

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestEnsureDir_ExistingDir(t *testing.T) {
	dir := t.TempDir()
	result, err := EnsureDir(dir)
	require.NoError(t, err)
	assert.Equal(t, dir, result)
}

func TestSafeFilename(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hello", "hello"},
		{"hello world", "hello world"},
		{`a<b>c:d"e`, "a_b_c_d_e"},
		{"file/with\\slash", "file_with_slash"},
		{"a|b?c*d", "a_b_c_d"},
		{"  spaces  ", "spaces"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, SafeFilename(tt.input))
		})
	}
}

func TestTruncateString(t *testing.T) {
	assert.Equal(t, "hello", TruncateString("hello", 10, "..."))
	assert.Equal(t, "hello", TruncateString("hello", 5, "..."))
	assert.Equal(t, "he...", TruncateString("hello world", 5, "..."))
	assert.Equal(t, "hel…", TruncateString("hello world", 6, "…")) // "…" is 3 bytes UTF-8
}

func TestTruncateString_EmptySuffix(t *testing.T) {
	assert.Equal(t, "he...", TruncateString("hello world", 5, ""))
}

func TestParseSessionKey_Valid(t *testing.T) {
	ch, id, err := ParseSessionKey("telegram:12345")
	require.NoError(t, err)
	assert.Equal(t, "telegram", ch)
	assert.Equal(t, "12345", id)
}

func TestParseSessionKey_WithColonInChatID(t *testing.T) {
	ch, id, err := ParseSessionKey("discord:guild:channel")
	require.NoError(t, err)
	assert.Equal(t, "discord", ch)
	assert.Equal(t, "guild:channel", id)
}

func TestParseSessionKey_Invalid(t *testing.T) {
	_, _, err := ParseSessionKey("nocolon")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid session key")
}

func TestTimestamp(t *testing.T) {
	ts := Timestamp()
	assert.NotEmpty(t, ts)
	assert.Contains(t, ts, "T") // ISO 8601 has T separator
}
