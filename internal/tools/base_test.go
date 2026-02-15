package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// RunToolContractTests runs the standard contract tests that ALL tools must pass.
// Call this in each tool's test file to ensure contract compliance.
func RunToolContractTests(t *testing.T, tool Tool) {
	t.Helper()

	t.Run("Contract/Name_NonEmpty", func(t *testing.T) {
		assert.NotEmpty(t, tool.Name(), "Tool.Name() must return non-empty string")
	})

	t.Run("Contract/Description_NonEmpty", func(t *testing.T) {
		assert.NotEmpty(t, tool.Description(), "Tool.Description() must return non-empty string")
	})

	t.Run("Contract/Parameters_ValidSchema", func(t *testing.T) {
		p := tool.Parameters()
		assert.NotNil(t, p, "Tool.Parameters() must not be nil")
		assert.Equal(t, "object", p["type"], "Parameters root type must be 'object'")
		_, hasProps := p["properties"]
		assert.True(t, hasProps, "Parameters must have 'properties' field")
	})

	t.Run("Contract/ToSchema_Format", func(t *testing.T) {
		schema := ToSchema(tool)
		assert.Equal(t, "function", schema["type"])
		fn, ok := schema["function"].(map[string]any)
		assert.True(t, ok)
		assert.Equal(t, tool.Name(), fn["name"])
		assert.Equal(t, tool.Description(), fn["description"])
	})
}
