package autopot

import (
	"context"
	"testing"
)

// TestRunStatusUIReturnsNilOnCancel verifies that runStatusUI returns nil
// when the context is cancelled immediately — signalling a normal Stop
// rather than triggering a fallback. The ctx.Err() check before pipeline
// init ensures Stop works even during initialisation.
func TestRunStatusUIReturnsNilOnCancel(t *testing.T) {
	a := NewAutoPot(AutoPotConfig{
		HPEnabled: true,
		HPKeyVK:   'W',
		Log:       func(string) {},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := a.runStatusUI(ctx, AutoPotConfig{Log: func(string) {}})
	if err != nil {
		t.Fatalf("runStatusUI with cancelled ctx: want nil, got %v", err)
	}
}
