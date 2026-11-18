package checkoutsessionuc

import (
	"testing"

	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	checkoutv2 "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/v2"
)

func TestCreatePartner_TrustedAssignment_SurfacesPayIn3Preview(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 3)
	h.expectActiveMerchant()
	h.expectKeyNotFound()

	previous := h.session(
		withStatus(model.ExpiredCheckoutSessionStatus),
		func(s *model.CheckoutSession) { s.IdempotencyKey = "attempt-0" },
	)
	h.sessionTx.EXPECT().
		LockByMerchantOrderID(h.ctx, h.merchantPID, testMerchantOrderID).
		Return(previous, nil).Once()

	created := h.expectNewAttemptCreated()
	unbound := created
	bound := created
	bound.CustomerID = &h.customerID
	h.expectPartnerCreateCustomerBind(unbound, bound)

	v2 := checkoutv2.New()
	assignment := model.PolicyAssignment{
		SubjectType:   model.CustomerPolicySubjectType,
		SubjectID:     h.profileID.String(),
		PolicyFamily:  model.CheckoutPolicyFamily,
		PolicyID:      v2.Ref().ID,
		PolicyVersion: v2.Ref().Version,
	}

	// Replace the harness default (no assignment → catalog v1) with trusted v2.
	h.policyAssignmentCQ.ExpectedCalls = nil
	h.policyAssignmentCQ.Calls = nil
	h.policyAssignmentCQ.EXPECT().
		GetActive(
			testifymock.Anything,
			model.CustomerPolicySubjectType,
			h.profileID.String(),
			model.CheckoutPolicyFamily,
			testifymock.Anything,
		).
		Return(assignment, nil)

	output, err := h.uc.CreatePartner(h.ctx, h.partnerRequest())
	require.NoError(t, err)
	require.Len(t, output.CreditKinds, 2)

	byKind := map[model.CreditKind]CreditKindPreview{}
	for _, preview := range output.CreditKinds {
		byKind[preview.Kind] = preview
	}

	installments := byKind[model.PayInInstallmentsCreditKind]
	require.Equal(t, model.PayIn3RepaymentPlan, installments.RepaymentPlan)
	require.Equal(t, 3, installments.InstallmentCount)
	// No credit account is provisioned in the harness, so availability stays false.
	require.False(t, installments.Available)
	require.NotNil(t, installments.Reason)
	require.Equal(t, "CREDIT_NOT_ASSIGNED", installments.Reason.Code)
}
