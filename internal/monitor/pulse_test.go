package monitor

import (
	"testing"
	"time"

	"github.com/agnivo988/Repo-lyzer/internal/github"
)

func TestCalculateLivePulse(t *testing.T) {
	now := time.Now()

	t.Run("Dormant Pulse", func(t *testing.T) {
		pulse := CalculateLivePulse([]github.Event{})
		if pulse != PulseDormant {
			t.Errorf("expected DORMANT, got %v", pulse)
		}
	})

	t.Run("Quiet Pulse", func(t *testing.T) {
		events := []github.Event{
			{CreatedAt: now.Add(-10 * time.Hour)}, // 1 event in last 24h
		}
		pulse := CalculateLivePulse(events)
		if pulse != PulseQuiet {
			t.Errorf("expected QUIET, got %v", pulse)
		}
	})

	t.Run("Spiking Pulse", func(t *testing.T) {
		events := make([]github.Event, 15)
		for i := 0; i < 15; i++ {
			events[i] = github.Event{CreatedAt: now.Add(-5 * time.Minute)}
		}

		pulse := CalculateLivePulse(events)
		if pulse != PulseSpiking {
			t.Errorf("expected SPIKING, got %v", pulse)
		}
	})
}
