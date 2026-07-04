package noop

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	extidentity "github.com/parstis/bnpl-backend/pkg/core/external/identity"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

var _ extidentity.Resolver = (*Client)(nil)

// subjectNamespace derives deterministic local subjects from mobile numbers.
var subjectNamespace = uuid.MustParse("6f0c3a56-8d52-4c43-9d51-1f3b3e0a7c10")

// Client resolves mobiles to deterministic local subjects without calling the
// Identity Service.
type Client struct{}

// New constructs a noop Identity Service client.
func New() *Client {
	return &Client{}
}

// ResolveByMobile returns a stable individual+active subject derived from the
// mobile, so the same mobile always resolves to the same IdentitySubjectID.
func (c *Client) ResolveByMobile(
	_ context.Context,
	input extidentity.ResolveByMobileInput,
) (extidentity.ResolveByMobileOutput, error) {
	const op = "noop.Client.ResolveByMobile"

	mobile := strings.TrimSpace(input.MobileE164)
	if mobile == "" {
		return extidentity.ResolveByMobileOutput{}, fmt.Errorf("%s: mobile is required", op)
	}

	profileID, err := model.ParseIdentitySubjectID(uuid.NewSHA1(subjectNamespace, []byte(mobile)).String())
	if err != nil {
		return extidentity.ResolveByMobileOutput{}, fmt.Errorf("%s: derive subject: %w", op, err)
	}

	return extidentity.ResolveByMobileOutput{
		IdentitySubjectID: profileID,
		SubjectType:       extidentity.IndividualSubjectType,
		SubjectStatus:     extidentity.ActiveSubjectStatus,
	}, nil
}
