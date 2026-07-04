package model

import (
	"bytes"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
)

func TestGenerateCheckoutSessionToken_Format(t *testing.T) {
	t.Parallel()

	token, err := GenerateCheckoutSessionToken(rand.Reader)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if len(token) != CheckoutSessionTokenLength {
		t.Fatalf("length = %d, want %d", len(token), CheckoutSessionTokenLength)
	}

	if !strings.HasPrefix(token.String(), CheckoutSessionTokenPrefix) {
		t.Fatalf("token %q missing prefix", token)
	}

	if err = token.Validate(); err != nil {
		t.Fatalf("generated token does not validate: %v", err)
	}
}

func TestGenerateCheckoutSessionToken_Deterministic(t *testing.T) {
	t.Parallel()

	source := bytes.NewReader(bytes.Repeat([]byte{0xab}, CheckoutSessionTokenRandomBytes))

	token, err := GenerateCheckoutSessionToken(source)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	want := "cs_" + strings.Repeat("ab", CheckoutSessionTokenRandomBytes)
	if token.String() != want {
		t.Fatalf("token = %q, want %q", token, want)
	}
}

func TestGenerateCheckoutSessionToken_Unique(t *testing.T) {
	t.Parallel()

	a, err := GenerateCheckoutSessionToken(rand.Reader)
	if err != nil {
		t.Fatalf("generate a: %v", err)
	}

	b, err := GenerateCheckoutSessionToken(rand.Reader)
	if err != nil {
		t.Fatalf("generate b: %v", err)
	}

	if a == b {
		t.Fatalf("two generated tokens collided: %q", a)
	}
}

func TestGenerateCheckoutSessionToken_ShortReader(t *testing.T) {
	t.Parallel()

	if _, err := GenerateCheckoutSessionToken(bytes.NewReader([]byte{1, 2, 3})); err == nil {
		t.Fatal("expected error for short random source")
	}

	if _, err := GenerateCheckoutSessionToken(nil); err == nil {
		t.Fatal("expected error for nil random source")
	}
}

func TestParseCheckoutSessionToken(t *testing.T) {
	t.Parallel()

	valid := "cs_" + strings.Repeat("0123456789abcdef", 4)

	tests := map[string]struct {
		input   string
		wantErr bool
	}{
		"valid":            {input: valid},
		"empty":            {input: "", wantErr: true},
		"missing prefix":   {input: strings.Repeat("0123456789abcdef", 4) + "abc", wantErr: true},
		"wrong prefix":     {input: "ct_" + strings.Repeat("0123456789abcdef", 4), wantErr: true},
		"uppercase hex":    {input: "cs_" + strings.Repeat("0123456789ABCDEF", 4), wantErr: true},
		"too short":        {input: valid[:len(valid)-1], wantErr: true},
		"too long":         {input: valid + "0", wantErr: true},
		"non hex":          {input: "cs_" + strings.Repeat("g", 64), wantErr: true},
		"leading space":    {input: " " + valid[:len(valid)-1], wantErr: true},
		"uuid":             {input: "8f0c2a4e-8a2b-4c1a-9b7e-1c2d3e4f5a6b", wantErr: true},
		"uppercase prefix": {input: "CS_" + strings.Repeat("0123456789abcdef", 4), wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			token, err := ParseCheckoutSessionToken(tc.input)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidCheckoutSessionToken) {
					t.Fatalf("err = %v, want ErrInvalidCheckoutSessionToken", err)
				}

				if !token.IsZero() {
					t.Fatalf("token = %q, want zero on error", token)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if token.String() != tc.input {
				t.Fatalf("token = %q, want %q", token, tc.input)
			}
		})
	}
}

func TestCheckoutSessionToken_IsZero(t *testing.T) {
	t.Parallel()

	var zero CheckoutSessionToken
	if !zero.IsZero() {
		t.Fatal("zero token must report IsZero")
	}

	if CheckoutSessionToken("cs_x").IsZero() {
		t.Fatal("non-empty token must not report IsZero")
	}
}
