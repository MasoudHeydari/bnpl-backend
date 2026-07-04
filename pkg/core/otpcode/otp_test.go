package otpcode

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
		"１２۳۴۵۶":  false,
	}

	for input, want := range tests {
		if got := IsWellFormed(input); got != want {
			t.Errorf("IsWellFormed(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestDomainSeparatedMaterial(t *testing.T) {
	t.Parallel()

	got := DomainSeparatedMaterial("hosted_checkout_confirm", "cs_abc", "3", "123456")
	want := "bnpl:otp:v2|23:hosted_checkout_confirm|6:cs_abc|1:3|6:123456"
	if got != want {
		t.Fatalf("material = %q, want %q", got, want)
	}

	if DomainSeparatedMaterial("hosted_checkout_confirm", "cs_a", "1", "123456") ==
		DomainSeparatedMaterial("hosted_checkout_confirm", "cs_b", "1", "123456") {
		t.Fatal("material must differ per binding")
	}

	if DomainSeparatedMaterial("hosted_checkout_confirm", "cs_a", "1", "123456") ==
		DomainSeparatedMaterial("hosted_checkout_confirm", "cs_a", "1", "654321") {
		t.Fatal("material must differ per code")
	}

	if DomainSeparatedMaterial("hosted_checkout_confirm", "cs_a", "1", "123456") ==
		DomainSeparatedMaterial("hosted_checkout_confirm", "cs_a", "2", "123456") {
		t.Fatal("material must differ per context version")
	}

	if DomainSeparatedMaterial("hosted_checkout_confirm", "cs_a", "1", "123456") ==
		DomainSeparatedMaterial("other_purpose", "cs_a", "1", "123456") {
		t.Fatal("material must differ per purpose")
	}
}

func TestDomainSeparatedMaterial_FieldBoundaryInjection(t *testing.T) {
	t.Parallel()

	// Classic separator ambiguity: ("a|b","c") vs ("a","|bc") must not collide.
	left := DomainSeparatedMaterial("a|b", "c", "1", "123456")
	right := DomainSeparatedMaterial("a", "|bc", "1", "123456")
	if left == right {
		t.Fatalf("boundary collision: both produced %q", left)
	}

	// Length-digit / payload ambiguity across empty and delimiter-containing fields.
	cases := [][2][4]string{
		{{"ab", "c", "1", "123456"}, {"a", "bc", "1", "123456"}},
		{{"", "x", "1", "123456"}, {"x", "", "1", "123456"}},
		{{"a:b", "c", "1", "123456"}, {"a", ":bc", "1", "123456"}},
		{{"12:ab", "c", "1", "123456"}, {"ab", "c", "1", "123456"}},
		{{"hosted_checkout_confirm", "tok|x", "1", "123456"}, {"hosted_checkout_confirm", "tok", "|x1", "123456"}},
	}

	for _, pair := range cases {
		a := DomainSeparatedMaterial(pair[0][0], pair[0][1], pair[0][2], pair[0][3])
		b := DomainSeparatedMaterial(pair[1][0], pair[1][1], pair[1][2], pair[1][3])
		if a == b {
			t.Fatalf("collision between %#v and %#v → %q", pair[0], pair[1], a)
		}
	}
}

func TestPhoneRateLimitMaterial(t *testing.T) {
	t.Parallel()

	got := PhoneRateLimitMaterial("+989121234567")
	if got != "bnpl:otp-rate-limit:phone:v1|+989121234567" {
		t.Fatalf("material = %q", got)
	}
}
