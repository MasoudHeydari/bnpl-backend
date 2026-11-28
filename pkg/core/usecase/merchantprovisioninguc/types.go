package merchantprovisioninguc

import "github.com/parstis/bnpl-backend/pkg/core/model"

// Input is the normalized command; the original envelope is retained in the inbox.
type Input struct {
	Event               model.InboxEvent
	MerchantID          model.ExternalMerchantID
	ApplicationID       model.MerchantApplicationID
	RequestID           model.MerchantParticipationRequestID
	Environment         string
	DeclarationRevision int64
}
