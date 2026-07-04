package checkoutotp_test

import (
	"testing"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/policy/checkoutotp"
)

func TestV1Terms(t *testing.T) {
	t.Parallel()

	got := checkoutotp.V1().Terms()

	want := checkoutotp.Terms{
		TTL:                     2 * time.Minute,
		MaxVerificationAttempts: 5,
		SendCooldown:            60 * time.Second,
		MaxSendsPerCheckout:     3,
		PhoneSendWindow:         10 * time.Minute,
		MaxSendsPerPhone:        5,
		MerchantSendWindow:      10 * time.Minute,
		MaxSendsPerMerchant:     30_000,
	}

	if got != want {
		t.Fatalf("V1().Terms() = %+v, want %+v", got, want)
	}

	if err := got.Validate(); err != nil {
		t.Fatalf("V1 terms must be valid: %v", err)
	}
}

func TestV1IsImmutableAcrossCalls(t *testing.T) {
	t.Parallel()

	first := checkoutotp.V1().Terms()
	first.MaxSendsPerCheckout = 99

	if got := checkoutotp.V1().Terms().MaxSendsPerCheckout; got != 3 {
		t.Fatalf("mutating a returned copy changed the policy: %d", got)
	}
}

func TestTermsValidate(t *testing.T) {
	t.Parallel()

	valid := checkoutotp.V1().Terms()

	tests := []struct {
		name   string
		mutate func(*checkoutotp.Terms)
	}{
		{name: "ttl", mutate: func(terms *checkoutotp.Terms) { terms.TTL = 0 }},
		{name: "attempts", mutate: func(terms *checkoutotp.Terms) { terms.MaxVerificationAttempts = 0 }},
		{name: "cooldown", mutate: func(terms *checkoutotp.Terms) { terms.SendCooldown = -time.Second }},
		{name: "sends", mutate: func(terms *checkoutotp.Terms) { terms.MaxSendsPerCheckout = 0 }},
		{name: "phone window", mutate: func(terms *checkoutotp.Terms) { terms.PhoneSendWindow = 0 }},
		{name: "phone sends", mutate: func(terms *checkoutotp.Terms) { terms.MaxSendsPerPhone = 0 }},
		{name: "merchant window", mutate: func(terms *checkoutotp.Terms) { terms.MerchantSendWindow = 0 }},
		{name: "merchant sends", mutate: func(terms *checkoutotp.Terms) { terms.MaxSendsPerMerchant = 0 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			terms := valid
			tt.mutate(&terms)

			if err := terms.Validate(); err == nil {
				t.Fatalf("Validate() accepted invalid %s", tt.name)
			}
		})
	}
}
