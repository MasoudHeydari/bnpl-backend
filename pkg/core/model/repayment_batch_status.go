package model

// RepaymentBatchStatus is BNPL's durable orchestration state for one customer
// cash collection. It is not the Financial Core provider or posting status.
type RepaymentBatchStatus string

const (
	CreatedRepaymentBatchStatus           RepaymentBatchStatus = "created"
	PreparedRepaymentBatchStatus          RepaymentBatchStatus = "prepared"
	StartingRepaymentBatchStatus          RepaymentBatchStatus = "starting"
	RequiresActionRepaymentBatchStatus    RepaymentBatchStatus = "requires_action"
	ReconcilingRepaymentBatchStatus       RepaymentBatchStatus = "reconciling"
	AccountingPendingRepaymentBatchStatus RepaymentBatchStatus = "accounting_pending"
	PostedRepaymentBatchStatus            RepaymentBatchStatus = "posted"
	FailedRepaymentBatchStatus            RepaymentBatchStatus = "failed"
	ExpiredRepaymentBatchStatus           RepaymentBatchStatus = "expired"
	ManualReviewRepaymentBatchStatus      RepaymentBatchStatus = "manual_review"
)

func (s RepaymentBatchStatus) IsValid() bool {
	switch s {
	case CreatedRepaymentBatchStatus, PreparedRepaymentBatchStatus, StartingRepaymentBatchStatus,
		RequiresActionRepaymentBatchStatus, ReconcilingRepaymentBatchStatus, AccountingPendingRepaymentBatchStatus,
		PostedRepaymentBatchStatus, FailedRepaymentBatchStatus, ExpiredRepaymentBatchStatus,
		ManualReviewRepaymentBatchStatus:
		return true
	default:
		return false
	}
}

func (s RepaymentBatchStatus) IsTerminal() bool {
	switch s {
	case PostedRepaymentBatchStatus, FailedRepaymentBatchStatus, ExpiredRepaymentBatchStatus:
		return true
	default:
		return false
	}
}
