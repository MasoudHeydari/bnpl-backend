package creditallocationrs

import (
	"strings"
	"testing"
)

func TestParseEligibilityJSON(t *testing.T) {
	const item = `{"national_code":"0012345678","mobile_e164":"+989120000001","group_code":"bank_employees"}`
	rows, err := parseEligibilityJSON(strings.NewReader("["+item+"]"), 1)
	if err != nil || len(rows) != 1 || rows[0].NationalCode != "0012345678" {
		t.Fatalf("unexpected result: %v", err)
	}
	for _, body := range []string{"[]", "null", item, "[" + item + "," + item + "]", "[" + item + "] {}", `[{"unknown":true}]`, "national_code,mobile_e164,group_code"} {
		if _, err := parseEligibilityJSON(strings.NewReader(body), 1); err == nil {
			t.Errorf("expected rejection: %s", body)
		}
	}
}
