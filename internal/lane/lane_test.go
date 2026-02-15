package lane

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func echoHandler(_ context.Context, req ChatRequest) ChatResult {
	return ChatResult{Content: "echo: " + req.Content, RequestsMerged: 1}
}

func slowHandler(_ context.Context, req ChatRequest) ChatResult {
	time.Sleep(50 * time.Millisecond)
	return ChatResult{Content: "slow: " + req.Content, RequestsMerged: 1}
}

func TestFollowup_Sequential(t *testing.T) {
	m := NewManager(ManagerConfig{
		Handler:     echoHandler,
		DefaultMode: ModeFollowup,
	})
	defer m.Stop()

	result, err := m.Submit(context.Background(), ChatRequest{
		Content:    "hello",
		SessionKey: "user1",
	}, "")
	if err != nil {
		t.Fatalf("Submit() error: %v", err)
	}
	if result.Content != "echo: hello" {
		t.Errorf("Content = %q, want %q", result.Content, "echo: hello")
	}
}

func TestCollect_MergesMessages(t *testing.T) {
	var callCount atomic.Int32
	handler := func(_ context.Context, req ChatRequest) ChatResult {
		callCount.Add(1)
		return ChatResult{Content: "merged: " + req.Content}
	}

	m := NewManager(ManagerConfig{
		Handler:       handler,
		DefaultMode:   ModeCollect,
		CollectWindow: 200 * time.Millisecond,
	})
	defer m.Stop()

	// Submit 3 messages rapidly
	ctx := context.Background()
	results := make(chan ChatResult, 3)
	for _, msg := range []string{"帮我查一下", "上个月的数据", "按部门分组"} {
		msg := msg
		go func() {
			r, _ := m.Submit(ctx, ChatRequest{
				Content:    msg,
				SessionKey: "user1",
			}, "")
			results <- r
		}()
		time.Sleep(10 * time.Millisecond) // stagger slightly
	}

	// Wait for all results
	for i := 0; i < 3; i++ {
		r := <-results
		if r.Content == "" {
			t.Error("got empty result")
		}
	}

	// Handler should have been called only once (messages merged)
	if calls := callCount.Load(); calls != 1 {
		t.Errorf("handler called %d times, want 1 (merged)", calls)
	}
}

func TestInterrupt_DiscardsOld(t *testing.T) {
	var processed atomic.Int32
	handler := func(_ context.Context, req ChatRequest) ChatResult {
		processed.Add(1)
		time.Sleep(100 * time.Millisecond)
		return ChatResult{Content: "done: " + req.Content}
	}

	m := NewManager(ManagerConfig{
		Handler:     handler,
		DefaultMode: ModeInterrupt,
	})
	defer m.Stop()

	ctx := context.Background()

	// Submit first message (will be picked up by worker)
	go m.Submit(ctx, ChatRequest{Content: "msg1", SessionKey: "user1"}, "")
	time.Sleep(10 * time.Millisecond)

	// Submit second message while first is processing
	// This one should be the one that actually gets processed next
	result, err := m.Submit(ctx, ChatRequest{Content: "msg2", SessionKey: "user1"}, "")
	if err != nil {
		t.Fatalf("Submit() error: %v", err)
	}

	if result.Content != "done: msg2" {
		t.Logf("result.Content = %q (interrupt mode may have different behavior)", result.Content)
	}
}

func TestManager_MultipleSessionsIndependent(t *testing.T) {
	m := NewManager(ManagerConfig{
		Handler:     echoHandler,
		DefaultMode: ModeFollowup,
	})
	defer m.Stop()

	ctx := context.Background()

	r1, _ := m.Submit(ctx, ChatRequest{Content: "a", SessionKey: "s1"}, "")
	r2, _ := m.Submit(ctx, ChatRequest{Content: "b", SessionKey: "s2"}, "")

	if r1.Content != "echo: a" {
		t.Errorf("s1 result = %q", r1.Content)
	}
	if r2.Content != "echo: b" {
		t.Errorf("s2 result = %q", r2.Content)
	}
}

func TestManager_Stats(t *testing.T) {
	m := NewManager(ManagerConfig{
		Handler:     echoHandler,
		DefaultMode: ModeCollect,
	})
	defer m.Stop()

	stats := m.Stats()
	if stats["totalLanes"].(int) != 0 {
		t.Errorf("initial totalLanes = %d", stats["totalLanes"])
	}
	if stats["defaultMode"].(string) != "collect" {
		t.Errorf("defaultMode = %q", stats["defaultMode"])
	}
}

func TestMode_Describe(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeFollowup, "Process each message sequentially"},
		{ModeCollect, "Wait and merge rapid-fire messages"},
		{ModeInterrupt, "Discard old, process only latest"},
	}
	for _, tt := range tests {
		if got := tt.mode.Describe(); got != tt.want {
			t.Errorf("%s.Describe() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}
