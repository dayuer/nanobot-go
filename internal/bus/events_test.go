package bus

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInboundMessage_SessionKey(t *testing.T) {
	msg := InboundMessage{Channel: "telegram", ChatID: "123"}
	assert.Equal(t, "telegram:123", msg.SessionKey())
}

func TestInboundMessage_SessionKey_Discord(t *testing.T) {
	msg := InboundMessage{Channel: "discord", ChatID: "guild_456"}
	assert.Equal(t, "discord:guild_456", msg.SessionKey())
}

func TestInboundMessage_JSON_RoundTrip(t *testing.T) {
	original := InboundMessage{
		Channel:   "telegram",
		SenderID:  "user1",
		ChatID:    "chat1",
		Content:   "hello",
		Timestamp: time.Now().Truncate(time.Second),
		Media:     []string{"https://example.com/img.png"},
		Metadata:  map[string]any{"key": "value"},
	}

	data, err := json.Marshal(original)
	assert.NoError(t, err)

	var decoded InboundMessage
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)

	assert.Equal(t, original.Channel, decoded.Channel)
	assert.Equal(t, original.SenderID, decoded.SenderID)
	assert.Equal(t, original.Content, decoded.Content)
	assert.Equal(t, original.SessionKey(), decoded.SessionKey())
}

func TestOutboundMessage_JSON_RoundTrip(t *testing.T) {
	original := OutboundMessage{
		Channel: "slack",
		ChatID:  "C123",
		Content: "world",
	}

	data, err := json.Marshal(original)
	assert.NoError(t, err)

	var decoded OutboundMessage
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)

	assert.Equal(t, original.Channel, decoded.Channel)
	assert.Equal(t, original.Content, decoded.Content)
}
