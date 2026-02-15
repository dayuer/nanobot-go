package channels

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dayuer/nanobot-go/internal/bus"
)

// RunChannelContractTests runs the standard contract tests that ALL channels must pass.
func RunChannelContractTests(t *testing.T, ch Channel) {
	t.Helper()

	t.Run("Contract/Name_NonEmpty", func(t *testing.T) {
		assert.NotEmpty(t, ch.Name(), "Channel.Name() must return non-empty string")
	})
}

func TestBaseChannel_IsAllowed_EmptyList(t *testing.T) {
	b := &BaseChannel{AllowFrom: []string{}}
	assert.True(t, b.IsAllowed("anyone"))
}

func TestBaseChannel_IsAllowed_InList(t *testing.T) {
	b := &BaseChannel{AllowFrom: []string{"user1", "user2"}}
	assert.True(t, b.IsAllowed("user1"))
	assert.True(t, b.IsAllowed("user2"))
	assert.False(t, b.IsAllowed("user3"))
}

func TestBaseChannel_IsAllowed_PipeSeparated(t *testing.T) {
	b := &BaseChannel{AllowFrom: []string{"user1"}}
	assert.True(t, b.IsAllowed("user1|extra"))
	assert.False(t, b.IsAllowed("user3|user4"))
}

func TestBaseChannel_HandleMessage_Allowed(t *testing.T) {
	mb := bus.NewMessageBus()
	b := &BaseChannel{
		ChannelName: "test",
		Bus:         mb,
		AllowFrom:   []string{},
	}

	b.HandleMessage("user1", "chat1", "hello", nil, nil)
	assert.Equal(t, 1, mb.InboundSize())

	msg := <-mb.Inbound
	assert.Equal(t, "test", msg.Channel)
	assert.Equal(t, "hello", msg.Content)
}

func TestBaseChannel_HandleMessage_Denied(t *testing.T) {
	mb := bus.NewMessageBus()
	b := &BaseChannel{
		ChannelName: "test",
		Bus:         mb,
		AllowFrom:   []string{"allowed_user"},
	}

	b.HandleMessage("blocked_user", "chat1", "hello", nil, nil)
	assert.Equal(t, 0, mb.InboundSize())
}
