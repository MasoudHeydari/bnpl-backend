package customercreditrs

import (
	"encoding/json"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
	"net/http"
	"testing"
)

func TestPendingCreditRequestHasNoFabricatedDecision(t *testing.T) {
	output := customercredituc.AssignInitialLimitOutput{Result: customercredituc.AssignInitialLimitResultPending, RequestCreated: true, CreditRequest: &model.CreditRequestSummary{Status: model.PendingCreditRequestStatus}}
	status, code, _ := assignmentMetadata([]customercredituc.AssignInitialLimitOutput{output})
	if status != http.StatusAccepted || code != "CREDIT_REQUEST_RECEIVED" {
		t.Fatalf("unexpected status/code %d %s", status, code)
	}
	data, err := json.Marshal(toAssignInitialLimitResponse(output))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err = json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["credit_limit_decision"] != nil || m["credit_limit"] != nil || m["credit_request"] == nil {
		t.Fatalf("unexpected pending payload %s", data)
	}
}
