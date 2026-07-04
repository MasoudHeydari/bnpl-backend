package checkoutotp

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestGenerateSixDigitOTP_Format(t *testing.T) {
	t.Parallel()

	for i := 0; i < 200; i++ {
		code, err := GenerateSixDigitOTP(rand.Reader)
		if err != nil {
			t.Fatalf("generate: %v", err)
		}

		if !IsWellFormed(code) {
			t.Fatalf("code %q is not six digits", code)
		}
	}
}

func TestGenerateSixDigitOTP_ZeroPadded(t *testing.T) {
	t.Parallel()

	// rand.Int reads big-endian bytes for the bound; all-zero bytes yield 0.
	code, err := GenerateSixDigitOTP(bytes.NewReader(make([]byte, 64)))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if code != "000000" {
		t.Fatalf("code = %q, want 000000", code)
	}
}

func TestGenerateSixDigitOTP_RandomSourceErrors(t *testing.T) {
	t.Parallel()

	if _, err := GenerateSixDigitOTP(nil); err == nil {
		t.Fatal("expected error for nil random source")
	}

	if _, err := GenerateSixDigitOTP(bytes.NewReader(nil)); err == nil {
		t.Fatal("expected error for exhausted random source")
	}
}

func TestIsWellFormed(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"123456":  true,
		"000000":  true,
		"12345":   false,
		"1234567": false,
		"12345a":  false,
		" 12345":  false,
		"۱۲۳۴۵۶":  false,
		"":        false,
		"12 456":  false,
		"-12345":  false,
		"１２３４５６":  false,
	}

	for input, want := range tests {
		if got := IsWellFormed(input); got != want {
			t.Errorf("IsWellFormed(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestDomainSeparatedMaterial(t *testing.T) {
	t.Parallel()

	got := DomainSeparatedMaterial("cs_abc", 3, "123456")
	if got != "bnpl:checkout-otp:v2|cs_abc|3|123456" {
		t.Fatalf("material = %q", got)
	}

	if DomainSeparatedMaterial("cs_a", 1, "123456") == DomainSeparatedMaterial("cs_b", 1, "123456") {
		t.Fatal("material must differ per checkout token")
	}

	if DomainSeparatedMaterial("cs_a", 1, "123456") == DomainSeparatedMaterial("cs_a", 1, "654321") {
		t.Fatal("material must differ per code")
	}

	if DomainSeparatedMaterial("cs_a", 1, "123456") == DomainSeparatedMaterial("cs_a", 2, "123456") {
		t.Fatal("material must differ per evaluation generation")
	}
}
