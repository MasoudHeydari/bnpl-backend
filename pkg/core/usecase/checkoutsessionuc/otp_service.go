package checkoutsessionuc

import (
	"context"

	otppolicy "github.com/parstis/bnpl-backend/pkg/core/policy/otp"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/otp"
)

// otpService is the narrow OTP surface Hosted Checkout depends on.
//
// Importing otp types here is intentional: otp must not import checkoutsessionuc.
type otpService interface {
	Request(ctx context.Context, input otp.RequestInput) (otp.RequestOutput, error)
	VerifyAndConsumeInTx(
		ctx context.Context,
		tx repo.Tx,
		input otp.VerifyInput,
	) (otp.VerifyOutput, error, error)
	Availability(ctx context.Context, input otp.AvailabilityInput) (otp.AvailabilityOutput, error)
	Terms() otppolicy.Terms
}
