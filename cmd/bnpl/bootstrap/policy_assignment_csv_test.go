package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestReadTrustedCustomersCSV(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "trusted-customers.csv")
	const body = "external_customer_id\n550e8400-e29b-41d4-a716-446655440000\n92d2c7b2-6f8b-4f25-8bb2-4deab49af151\n"
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
	want, err := model.ParseExternalCustomerID("550e8400-e29b-41d4-a716-446655440000")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].ExternalCustomerID != want {
		t.Fatalf("customer id=%s; want %s", rows[0].ExternalCustomerID, want)
	}
}

func TestReadTrustedCustomersCSVRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{"legacy_header", "subject_id\n550e8400-e29b-41d4-a716-446655440000\n", "header must be exactly: external_customer_id"},
		{"internal_customer_column", "customer_id\n550e8400-e29b-41d4-a716-446655440000\n", "header must be exactly: external_customer_id"},
		{"non_uuid", "external_customer_id\nnot-an-uuid\n", "invalid external_customer_id"},
		{"zero_uuid", "external_customer_id\n00000000-0000-0000-0000-000000000000\n", "invalid external_customer_id"},
		{"missing_row", "external_customer_id\n", "contains no data rows"},
		{"empty_id", "external_customer_id\n\n", "contains no data rows"},
		{"extra_column", "external_customer_id,other\n550e8400-e29b-41d4-a716-446655440000,extra\n", "header must be exactly: external_customer_id"},
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
