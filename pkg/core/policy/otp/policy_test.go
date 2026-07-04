package otp_test

import (
	"testing"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/policy/otp"
)

func TestV1Terms(t *testing.T) {
	t.Parallel()

	got := otp.V1().Terms()

	want := otp.Terms{
		TTL:                     2 * time.Minute,
		MaxVerificationAttempts: 5,
		SendCooldown:            60 * time.Second,
		MaxSendsPerSubject:      3,
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

	first := otp.V1().Terms()
	first.MaxSendsPerSubject = 99

	if got := otp.V1().Terms().MaxSendsPerSubject; got != 3 {
		t.Fatalf("mutating a returned copy changed the policy: %d", got)
	}
}

func TestTermsValidate(t *testing.T) {
	t.Parallel()

	valid := otp.V1().Terms()

	tests := []struct {
		name   string
		mutate func(*otp.Terms)
	}{
		{name: "ttl", mutate: func(terms *otp.Terms) { terms.TTL = 0 }},
		{name: "attempts", mutate: func(terms *otp.Terms) { terms.MaxVerificationAttempts = 0 }},
		{name: "cooldown", mutate: func(terms *otp.Terms) { terms.SendCooldown = -time.Second }},
		{name: "sends", mutate: func(terms *otp.Terms) { terms.MaxSendsPerSubject = 0 }},
		{name: "phone window", mutate: func(terms *otp.Terms) { terms.PhoneSendWindow = 0 }},
		{name: "phone sends", mutate: func(terms *otp.Terms) { terms.MaxSendsPerPhone = 0 }},
		{name: "merchant window", mutate: func(terms *otp.Terms) { terms.MerchantSendWindow = 0 }},
		{name: "merchant sends", mutate: func(terms *otp.Terms) { terms.MaxSendsPerMerchant = 0 }},
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
