//go:build linux

package privilege

import (
	"errors"
	"fmt"
	"os"
)

// IsElevated returns true when the current process executes with root privileges.
func IsElevated() bool {
	return os.Geteuid() == 0
}

// AttemptElevate informs the caller that automatic elevation is not supported on Linux.
// Returns (relaunched, error). Since we cannot relaunch ourselves with sudo, we
// always return false along with an instructive error.
func AttemptElevate() (bool, error) {
	if IsElevated() {
		return false, nil
	}
	return false, errors.New("elevation not supported automatically; please rerun the agent with sudo")
}

// CurrentUserHint returns a message that can be shown to the operator when elevation fails.
func CurrentUserHint() string {
	return fmt.Sprintf("current uid=%d (need root to access privileged operations)", os.Geteuid())
}
