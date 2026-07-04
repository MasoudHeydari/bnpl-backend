package checkoutsessionuc

import "testing"

func TestMaskCustomerPhone(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"+989121234567": "0912***4567",
		"+989001110000": "0900***0000",
		"":              "***",
		"09121234567":   "***",
		"+14155552671":  "***",
		"+98912":        "***",
		"+98912abc4567": "***",
	}

	for input, want := range tests {
		if got := MaskCustomerPhone(input); got != want {
			t.Errorf("MaskCustomerPhone(%q) = %q, want %q", input, got, want)
		}
	}
}
