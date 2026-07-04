package config

import (
	"testing"
	"time"
)

func TestShutdownTimeoutOrDefault(t *testing.T) {
	t.Parallel()

	if got := (Shutdown{}).getShutdownTimeout(); got != defaultShutdownTimeout {
		t.Fatalf("default timeout = %s, want %s", got, defaultShutdownTimeout)
	}

	custom := Shutdown{Timeout: defaultShutdownTimeout + 1}
	if got := custom.getShutdownTimeout(); got != custom.Timeout {
		t.Fatalf("custom timeout = %s, want %s", got, custom.Timeout)
	}
}

func TestShutdownDrainDelayOrDefault(t *testing.T) {
	t.Parallel()

	if got := (Shutdown{}).getShutdownDrainDelay(); got != 0 {
		t.Fatalf("zero drain delay = %s, want 0", got)
	}

	custom := Shutdown{DrainDelay: defaultShutdownDrainDelay + 1}
	if got := custom.getShutdownDrainDelay(); got != custom.DrainDelay {
		t.Fatalf("custom drain delay = %s, want %s", got, custom.DrainDelay)
	}

	negative := Shutdown{DrainDelay: -time.Second}
	if got := negative.getShutdownDrainDelay(); got != defaultShutdownDrainDelay {
		t.Fatalf("negative drain delay = %s, want default %s", got, defaultShutdownDrainDelay)
	}
}
