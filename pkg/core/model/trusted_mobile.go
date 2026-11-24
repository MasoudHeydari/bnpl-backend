package model

import "time"

// TrustedMobileNumber identifies a raw phone/national ID pair from an operator
// import. Membership is not a credit grant: CRS must verify the pair before
// initial credit assignment. The phone may later change at Customer Service.
type TrustedMobileNumber struct {
	MobileE164 string    `db:"mobile_e164"`
	NationalID string    `db:"national_id"`
	CreatedBy  string    `db:"created_by"`
	CreatedAt  time.Time `db:"created_at"`
}
