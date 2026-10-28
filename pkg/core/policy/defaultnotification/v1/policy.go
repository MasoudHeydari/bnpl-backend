// Package v1 defines truthful default-entry notification timing independently
// of the policy that establishes default. Durable request deduplication is an
// orchestration concern; delivery is owned by Notification Service.
package v1

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const TemplateID = "bnpl.collections.default.v1"
const PolicyVersion = "collections_default_notification.v1"

// Eligible includes catch-up after an outage, but only after 13:00 on a business
// day, while authoritative unpaid evidence remains and uncertainty is absent.
func Eligible(now, boundary time.Time, location *time.Location, active, applicable, uncertain bool) bool {
	if !active || !applicable || uncertain || boundary.IsZero() {
		return false
	}
	start := boundary.In(location)
	window := time.Date(start.Year(), start.Month(), start.Day(), 13, 0, 0, 0, location)
	return !now.Before(boundary) && !now.Before(window) && now.In(location).Hour() >= 13
}

// Eligibility is a read-only notification decision inside the customer's lock.
type Eligibility struct {
	Reserved       bool
	Eligible       bool
	EpisodeID      model.DefaultEpisodeID
	InstallmentIDs []model.InstallmentID
}
