package model_test

import (
	"bytes"
	"testing"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestGenerateOTPIssueID(t *testing.T) {
	t.Parallel()

	id, err := model.GenerateOTPIssueID(bytes.NewReader(bytes.Repeat([]byte{0xab}, 16)))
	if err != nil {
		t.Fatalf("GenerateOTPIssueID: %v", err)
	}

	if err := id.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	want := model.OTPIssueID("oi_" + "abababababababababababababababab")
	if id != want {
		t.Fatalf("got %q, want %q", id, want)
	}
}

func TestOTPIssueIDValidateRejectsMalformed(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		"",
		"oi_short",
		"oi_ABCDEF0123456789abcdef0123456789",
		"cs_0123456789abcdef0123456789abcdef",
	} {
		if err := model.OTPIssueID(value).Validate(); err == nil {
			t.Fatalf("Validate(%q) accepted malformed id", value)
		}
	}
}
