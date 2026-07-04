package customerrs

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customeraccountuc"
	"github.com/stretchr/testify/require"
)

func TestRepaymentSerializationSeparatesOriginalAndCompletePayable(t *testing.T) {
	amount := uint64(115)
	id := model.InstallmentID(uuid.New())
	contract := model.ContractID(uuid.New())
	data := toUnpaidInstallmentsListData(customeraccountuc.ListUnpaidInstallmentsOutput{
		Installments: []customeraccountuc.UnpaidInstallment{{ID: id, ContractID: contract, TotalAmountMinor: 130, CustomerPayableAmountMinor: &amount, Status: model.DueInstallmentStatus}},
		Repayment:    customeraccountuc.RepaymentSummary{DueInstallments: []customeraccountuc.RepaymentDueInstallment{{ID: id, ContractID: contract, TotalAmountMinor: 130, CustomerPayableAmountMinor: &amount}}, NextPayable: &customeraccountuc.RepaymentNextPayable{InstallmentID: id, ContractID: contract, PayableAmountMinor: 115, OutstandingAmountMinor: 115, PayableAmountKnown: true, HasPendingPayment: true}},
	})
	raw, e := json.Marshal(data)
	require.NoError(t, e)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	installments := decoded["installments"].([]any)
	row := installments[0].(map[string]any)
	require.Equal(t, float64(130), row["total_amount_minor"])
	require.Equal(t, float64(115), row["customer_payable_amount_minor"])
	require.NotContains(t, string(raw), "partial")
}
