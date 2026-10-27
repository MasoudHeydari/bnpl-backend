package config

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestReminderShortLinkConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, host, destination string
		enabled, wantError      bool
	}{
		{name: "disabled needs no fake configuration"},
		{name: "enabled missing configuration", enabled: true, wantError: true},
		{name: "valid", host: "s.example.com", destination: "https://pay.example.com/installments", enabled: true},
		{name: "host contains scheme", host: "https://s.example.com", destination: "https://pay.example.com/installments", enabled: true, wantError: true},
		{name: "host contains path", host: "s.example.com/path", destination: "https://pay.example.com/installments", enabled: true, wantError: true},
		{name: "insecure", host: "s.example.com", destination: "http://pay.example.com/installments", enabled: true, wantError: true},
		{name: "unapproved destination", host: "s.example.com", destination: "https://elsewhere.example/installments", enabled: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cfg Config
			cfg.Infrastructure.Task.Scheduled.RepaymentReminderDispatch.Enabled = tc.enabled
			cfg.Usecases.ShortLink.PublicHost = tc.host
			cfg.Usecases.ShortLink.RepaymentDestinationURL = tc.destination
			cfg.Usecases.ShortLink.AllowedDestinationHosts = []string{"pay.example.com"}
			err := cfg.validateRepaymentReminderShortLink()
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
