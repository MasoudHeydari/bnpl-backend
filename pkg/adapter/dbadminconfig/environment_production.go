//go:build production

package dbadminconfig

import "fmt"

func validateBuildEnvironment(env string) error {
	if env != "production" && env != "staging" {
		return fmt.Errorf("production dbadmin requires production or staging environment")
	}
	return nil
}
