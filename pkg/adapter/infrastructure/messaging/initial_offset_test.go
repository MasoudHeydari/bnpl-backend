package messaging

import "testing"

func TestParseInitialOffset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    InitialOffset
		wantErr bool
	}{
		{name: "empty defaults to oldest", input: "", want: InitialOffsetOldest},
		{name: "whitespace defaults to oldest", input: "  ", want: InitialOffsetOldest},
		{name: "oldest", input: "oldest", want: InitialOffsetOldest},
		{name: "newest", input: "newest", want: InitialOffsetNewest},
		{name: "mixed case", input: "NeWeSt", want: InitialOffsetNewest},
		{name: "invalid", input: "middle", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseInitialOffset(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseInitialOffset(%q) error = %v", tt.input, err)
			}

			if got != tt.want {
				t.Fatalf("ParseInitialOffset(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
