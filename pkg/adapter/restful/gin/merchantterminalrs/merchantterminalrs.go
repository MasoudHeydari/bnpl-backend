// Package merchantterminalrs exposes Merchant Admin Panel terminal credential
// management over HTTP.
//
// Provision, Rotate, Retire, and Get are panel operations authenticated by the
// gateway AuthContext (merchant session/token). They do NOT use
// middleware.PartnerTerminalAuth or the X-Terminal-Id / X-API-Key headers;
// those authenticate only /api/v1/partner/** runtime integration calls.
//
// API key show-once policy: plaintext api_key is returned only by the issuance
// response (Provision/Rotate) and by an idempotent replay of that same
// issuance request within the configured replay window. GET never exposes the
// secret because BNPL stores only an irreversible verifier. The merchant must
// copy the key into their own secure store when it is issued. Lost keys require
// Rotate (use grace_period_seconds=0 for immediate retirement of the previous
// key). Responses that carry a plaintext API key are marked Cache-Control:
// no-store and the key is never logged.
//
// Partner runtime permissions are persisted on the terminal at Provision and
// loaded on Authenticate; they are not encoded in the API key and are not
// changed by Rotate/Retire.
package merchantterminalrs
