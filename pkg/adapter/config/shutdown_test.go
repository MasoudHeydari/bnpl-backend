package config

import "testing"

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

	if got := (Shutdown{}).getShutdownDrainDelay(); got != defaultShutdownDrainDelay {
		t.Fatalf("default drain delay = %s, want %s", got, defaultShutdownDrainDelay)
	}

	custom := Shutdown{DrainDelay: defaultShutdownDrainDelay + 1}
	if got := custom.getShutdownDrainDelay(); got != custom.DrainDelay {
		t.Fatalf("custom drain delay = %s, want %s", got, custom.DrainDelay)
	}
}
