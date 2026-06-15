package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"gopkg.in/yaml.v3"
)

func hashOf(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func TestNewSecret(t *testing.T) {
	secret := NewSecret("my-password")

	if got := secret.Reveal(); got != "my-password" {
		t.Errorf("Reveal() = %q, want %q", got, "my-password")
	}
	if got := secret.Hash(); got != hashOf("my-password") {
		t.Errorf("Hash() = %q, want %q", got, hashOf("my-password"))
	}
	if secret.IsEmpty() {
		t.Error("IsEmpty() = true, want false")
	}
}

func TestSecret_Reveal(t *testing.T) {
	secret := NewSecret("plaintext")
	if got := secret.Reveal(); got != "plaintext" {
		t.Errorf("Reveal() = %q, want %q", got, "plaintext")
	}
}

func TestSecret_Verify(t *testing.T) {
	secret := NewSecret("correct-password")

	if !secret.Verify("correct-password") {
		t.Error("Verify() = false for matching password, want true")
	}
	if secret.Verify("wrong-password") {
		t.Error("Verify() = true for non-matching password, want false")
	}
}

func TestSecret_IsEmpty(t *testing.T) {
	empty := Secret{}
	if !empty.IsEmpty() {
		t.Error("zero Secret IsEmpty() = false, want true")
	}

	nonEmpty := NewSecret("value")
	if nonEmpty.IsEmpty() {
		t.Error("NewSecret IsEmpty() = true, want false")
	}
}

func TestSecret_Clear(t *testing.T) {
	secret := NewSecret("to-clear")
	secret.Clear()

	if got := secret.Reveal(); got != "" {
		t.Errorf("Reveal() after Clear = %q, want empty", got)
	}
	if got := secret.Hash(); got != "" {
		t.Errorf("Hash() after Clear = %q, want empty", got)
	}
	if !secret.IsEmpty() {
		t.Error("IsEmpty() after Clear = false, want true")
	}
}

func TestSecret_Equal(t *testing.T) {
	a := NewSecret("same")
	b := NewSecret("same")
	c := NewSecret("different")

	if !a.Equal(b) {
		t.Error("Equal() = false for same value, want true")
	}
	if a.Equal(c) {
		t.Error("Equal() = true for different values, want false")
	}
}

func TestSecret_String(t *testing.T) {
	secret := NewSecret("hidden")
	want := fmt.Sprintf("Secret{%s...}", hashOf("hidden")[:16])
	if got := secret.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}

	empty := Secret{}
	if got := empty.String(); got != "" {
		t.Errorf("String() on empty = %q, want empty", got)
	}
}

func TestSecret_GoString(t *testing.T) {
	secret := NewSecret("hidden")
	if got, want := secret.GoString(), secret.String(); got != want {
		t.Errorf("GoString() = %q, want %q", got, want)
	}
}

func TestSecret_Format(t *testing.T) {
	secret := NewSecret("hidden")
	hashPrefix := hashOf("hidden")[:16]

	tests := []struct {
		name   string
		format string
		want   string
	}{
		{"default verb", "%v", fmt.Sprintf("Secret{%s...}", hashPrefix)},
		{"string verb", "%s", fmt.Sprintf("Secret{%s...}", hashPrefix)},
		{"go syntax", "%#v", fmt.Sprintf("model.Secret{hash: %s...}", hashPrefix)},
		{"detailed", "%+v", fmt.Sprintf("Secret{hash: %s, length: %d}", hashPrefix, len("hidden"))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fmt.Sprintf(tt.format, secret); got != tt.want {
				t.Errorf("fmt.Sprintf(%q) = %q, want %q", tt.format, got, tt.want)
			}
		})
	}
}

func TestSecret_MarshalUnmarshalJSON(t *testing.T) {
	original := NewSecret("json-secret")

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("MarshalJSON() error: %v", err)
	}

	wantJSON := fmt.Sprintf(`"REDACTED[%s...]"`, hashOf("json-secret")[:16])
	if string(data) != wantJSON {
		t.Errorf("MarshalJSON() = %s, want %s", data, wantJSON)
	}

	var decoded Secret
	if err := json.Unmarshal([]byte(`"json-secret"`), &decoded); err != nil {
		t.Fatalf("UnmarshalJSON() error: %v", err)
	}
	if !original.Equal(decoded) {
		t.Error("Unmarshaled secret does not equal original")
	}
}

func TestSecret_MarshalJSON_empty(t *testing.T) {
	data, err := json.Marshal(Secret{})
	if err != nil {
		t.Fatalf("MarshalJSON() on empty error: %v", err)
	}
	if string(data) != `""` {
		t.Errorf("MarshalJSON() on empty = %s, want %q", data, `""`)
	}
}

func TestSecret_UnmarshalJSON_invalid(t *testing.T) {
	var s Secret
	err := s.UnmarshalJSON([]byte(`123`))
	if err == nil {
		t.Fatal("UnmarshalJSON() with non-string input: expected error")
	}
}

func TestSecret_MarshalUnmarshalYAML(t *testing.T) {
	original := NewSecret("yaml-secret")

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("MarshalYAML() error: %v", err)
	}

	wantYAML := fmt.Sprintf("REDACTED[%s...]\n", hashOf("yaml-secret")[:16])
	if string(data) != wantYAML {
		t.Errorf("MarshalYAML() = %q, want %q", data, wantYAML)
	}

	var decoded Secret
	if err := yaml.Unmarshal([]byte("yaml-secret"), &decoded); err != nil {
		t.Fatalf("UnmarshalYAML() error: %v", err)
	}
	if !original.Equal(decoded) {
		t.Error("Unmarshaled secret does not equal original")
	}
}

func TestSecret_MarshalYAML_empty(t *testing.T) {
	data, err := yaml.Marshal(Secret{})
	if err != nil {
		t.Fatalf("MarshalYAML() on empty error: %v", err)
	}
	if string(data) != `""`+"\n" {
		t.Errorf("MarshalYAML() on empty = %q, want %q", data, `""`+"\n")
	}
}

func TestSecret_UnmarshalYAML_invalid(t *testing.T) {
	var s Secret
	err := yaml.Unmarshal([]byte("key: value"), &s)
	if err == nil {
		t.Fatal("UnmarshalYAML() with mapping input: expected error")
	}
}

func TestSecret_doesNotLeakPlaintext(t *testing.T) {
	secret := NewSecret("super-secret-value")
	plaintext := "super-secret-value"

	checks := []string{
		secret.String(),
		secret.GoString(),
		fmt.Sprintf("%v", secret),
		fmt.Sprintf("%#v", secret),
		fmt.Sprintf("%+v", secret),
	}

	data, err := json.Marshal(secret)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	checks = append(checks, string(data))

	yamlData, err := yaml.Marshal(secret)
	if err != nil {
		t.Fatalf("yaml.Marshal() error: %v", err)
	}
	checks = append(checks, string(yamlData))

	for _, got := range checks {
		if got == plaintext {
			t.Errorf("output %q contains plaintext secret", got)
		}
	}
}
