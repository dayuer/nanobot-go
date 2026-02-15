package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Registry Lookup Tests ---

func TestFindByModel_Anthropic(t *testing.T) {
	spec := FindByModel("anthropic/claude-sonnet-4-5")
	require.NotNil(t, spec)
	assert.Equal(t, "anthropic", spec.Name)
}

func TestFindByModel_Claude(t *testing.T) {
	spec := FindByModel("claude-3-haiku-20240307")
	require.NotNil(t, spec)
	assert.Equal(t, "anthropic", spec.Name)
}

func TestFindByModel_DeepSeek(t *testing.T) {
	spec := FindByModel("deepseek-chat")
	require.NotNil(t, spec)
	assert.Equal(t, "deepseek", spec.Name)
}

func TestFindByModel_Gemini(t *testing.T) {
	spec := FindByModel("gemini-pro")
	require.NotNil(t, spec)
	assert.Equal(t, "gemini", spec.Name)
}

func TestFindByModel_Qwen(t *testing.T) {
	spec := FindByModel("qwen-max")
	require.NotNil(t, spec)
	assert.Equal(t, "dashscope", spec.Name)
}

func TestFindByModel_Unknown(t *testing.T) {
	spec := FindByModel("some-unknown-model")
	assert.Nil(t, spec)
}

func TestFindByModel_SkipsGateways(t *testing.T) {
	// "openrouter" keyword exists but is a gateway; FindByModel skips it
	spec := FindByModel("openrouter/something")
	// Should not match openrouter (gateway), only if it matches a standard provider
	assert.True(t, spec == nil || !spec.IsGateway)
}

func TestFindGateway_ByName(t *testing.T) {
	spec := FindGateway("openrouter", "", "")
	require.NotNil(t, spec)
	assert.Equal(t, "openrouter", spec.Name)
	assert.True(t, spec.IsGateway)
}

func TestFindGateway_ByName_VLLMLocal(t *testing.T) {
	spec := FindGateway("vllm", "", "")
	require.NotNil(t, spec)
	assert.Equal(t, "vllm", spec.Name)
	assert.True(t, spec.IsLocal)
}

func TestFindGateway_ByKeyPrefix(t *testing.T) {
	spec := FindGateway("", "sk-or-abc123", "")
	require.NotNil(t, spec)
	assert.Equal(t, "openrouter", spec.Name)
}

func TestFindGateway_ByBaseKeyword(t *testing.T) {
	spec := FindGateway("", "", "https://aihubmix.com/v1")
	require.NotNil(t, spec)
	assert.Equal(t, "aihubmix", spec.Name)
}

func TestFindGateway_NoMatch(t *testing.T) {
	spec := FindGateway("", "sk-normal-key", "https://api.example.com")
	assert.Nil(t, spec)
}

func TestFindGateway_StandardProviderNotGateway(t *testing.T) {
	// DeepSeek is a standard provider, not a gateway
	spec := FindGateway("deepseek", "", "")
	assert.Nil(t, spec)
}

func TestFindByName(t *testing.T) {
	spec := FindByName("moonshot")
	require.NotNil(t, spec)
	assert.Equal(t, "Moonshot", spec.DisplayName)
	assert.Equal(t, "moonshot", spec.LiteLLMPrefix)
}

func TestFindByName_NotFound(t *testing.T) {
	spec := FindByName("nonexistent")
	assert.Nil(t, spec)
}

func TestProviderSpec_Label(t *testing.T) {
	spec := &ProviderSpec{Name: "test", DisplayName: "Test Provider"}
	assert.Equal(t, "Test Provider", spec.Label())

	spec2 := &ProviderSpec{Name: "test"}
	assert.Equal(t, "Test", spec2.Label())
}

func TestProviderSpec_ModelOverrides(t *testing.T) {
	spec := FindByName("moonshot")
	require.NotNil(t, spec)
	require.Len(t, spec.ModelOverrides, 1)
	assert.Equal(t, "kimi-k2.5", spec.ModelOverrides[0].Pattern)
	assert.Equal(t, 1.0, spec.ModelOverrides[0].Overrides["temperature"])
}

// --- Provider Count ---

func TestProviderCount(t *testing.T) {
	assert.Len(t, Providers, 13, "should match upstream's 13 providers (incl. custom)")
}
