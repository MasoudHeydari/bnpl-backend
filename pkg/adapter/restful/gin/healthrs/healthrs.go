// Package healthrs exposes HTTP health-check resources for Gin.
//
// It registers lightweight liveness and readiness endpoints used by
// orchestrators such as Kubernetes to decide whether the process is alive and
// ready to receive traffic. The package translates HTTP requests to the
// health-check use case and serializes the result without owning dependency
// checking logic itself.
package healthrs
