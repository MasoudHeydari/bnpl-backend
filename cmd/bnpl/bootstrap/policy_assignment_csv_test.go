package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadTrustedCustomersCSV(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "trusted-customers.csv")
	const body = "mobile_e164\n+989121234567\n09191234567\n"
	if err := os.WriteFile(file, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}

	rows, err := readTrustedCustomersCSV(file)
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 2 {
		t.Fatalf("rows=%d; want 2", len(rows))
	}

	if rows[0].RowNumber != 2 || rows[1].RowNumber != 3 {
		t.Fatalf("incorrect CSV row numbers: %+v", rows)
	}

	if rows[0].MobileE164 != "+989121234567" {
		t.Fatalf("mobile=%q; want +989121234567", rows[0].MobileE164)
	}

	if rows[1].MobileE164 != "09191234567" {
		t.Fatalf("mobile=%q; want 09191234567", rows[1].MobileE164)
	}
}

func TestReadTrustedCustomersCSVRejectsInvalidHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{"legacy_external_customer_id", "external_customer_id\n550e8400-e29b-41d4-a716-446655440000\n", "header must be exactly: mobile_e164"},
		{"subject_id", "subject_id\n+989121234567\n", "header must be exactly: mobile_e164"},
		{"missing_row", "mobile_e164\n", "contains no data rows"},
		{"extra_header_column", "mobile_e164,other\n+989121234567,extra\n", "header must be exactly: mobile_e164"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "trusted-customers.csv")
			if err := os.WriteFile(path, []byte(tt.content), 0600); err != nil {
				t.Fatal(err)
			}

			_, err := readTrustedCustomersCSV(path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error=%v; expected substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestReadTrustedCustomersCSVKeepsMalformedRows(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "trusted-customers.csv")
	// encoding/csv ignores blank lines; keep an explicit empty field instead.
	const body = "mobile_e164\nnot-a-phone\n\"\"\n+989121234567\n"
	if err := os.WriteFile(file, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}

	rows, err := readTrustedCustomersCSV(file)
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 3 {
		t.Fatalf("rows=%d; want 3 (malformed rows kept for row-level handling)", len(rows))
	}
}
