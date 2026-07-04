package iran

import (
	"errors"
	"testing"

	"github.com/parstis/bnpl-backend/pkg/core/canonicalizer/phonenumber"
)

func TestCanonicalize_Accepts(t *testing.T) {
	t.Parallel()

	c := New()
	cases := map[string]string{
		"09121234567":   "+989121234567",
		"+989121234567": "+989121234567",
		"09001234567":   "+989001234567",
		"+989991234567": "+989991234567",
	}

	for input, want := range cases {
		got, err := c.Canonicalize(input)
		if err != nil {
			t.Errorf("Canonicalize(%q) error = %v", input, err)
			continue
		}

		if got != want {
			t.Errorf("Canonicalize(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCanonicalize_Rejects(t *testing.T) {
	t.Parallel()

	c := New()
	cases := []struct {
		name  string
		input string
	}{
		{name: "leading space", input: " 09121234567"},
		{name: "trailing space", input: "09121234567 "},
		{name: "both spaces", input: " 09121234567 "},
		{name: "inner space", input: "0912 123 4567"},
		{name: "dash", input: "0912-123-4567"},
		{name: "parens", input: "(0912)1234567"},
		{name: "persian digits", input: "۰۹۱۲۱۲۳۴۵۶۷"},
		{name: "arabic indic digits", input: "٠٩١٢١٢٣٤٥٦٧"},
		{name: "mixed persian", input: "0912123456۷"},
		{name: "fullwidth digits", input: "０９１２１２３４５６７"},
		{name: "0098 prefix", input: "00989121234567"},
		{name: "98 prefix", input: "989121234567"},
		{name: "bare subscriber", input: "9121234567"},
		{name: "landline e164", input: "+982112345678"},
		{name: "landline local", input: "02112345678"},
		{name: "us e164", input: "+14155552671"},
		{name: "too short local", input: "0912123456"},
		{name: "too long local", input: "091212345678"},
		{name: "short", input: "+98912123456"},
		{name: "long", input: "+9891212345678"},
		{name: "letters", input: "+98912abc567"},
		{name: "plus without 98", input: "+09121234567"},
		{name: "double plus", input: "++989121234567"},
		{name: "newline", input: "09121234567\n"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := c.Canonicalize(tt.input)
			if !errors.Is(err, phonenumber.ErrInvalid) {
				t.Errorf("Canonicalize(%q) = (%q, %v), want ErrInvalid", tt.input, got, err)
			}
		})
	}
}

func TestCanonicalize_Empty(t *testing.T) {
	t.Parallel()

	if _, err := New().Canonicalize(""); !errors.Is(err, phonenumber.ErrRequired) {
		t.Fatalf("Canonicalize(\"\") error = %v, want ErrRequired", err)
	}
}
