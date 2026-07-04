package sarama

import (
	"testing"

	"github.com/IBM/sarama"

	adaptermessaging "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging"
)

func TestMapInitialOffset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   adaptermessaging.InitialOffset
		want    int64
		wantErr bool
	}{
		{name: "oldest", input: adaptermessaging.InitialOffsetOldest, want: sarama.OffsetOldest},
		{name: "newest", input: adaptermessaging.InitialOffsetNewest, want: sarama.OffsetNewest},
		{name: "invalid", input: adaptermessaging.InitialOffset("middle"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := mapInitialOffset(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("mapInitialOffset(%q) error = %v", tt.input, err)
			}

			if got != tt.want {
				t.Fatalf("mapInitialOffset(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
