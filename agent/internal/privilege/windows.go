//go:build windows

package privilege

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// IsElevated tries a simple check by running a command that requires admin.
func IsElevated() bool {
	// Query admin group membership using whoami
	cmd := exec.Command("whoami", "/groups")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	s := strings.ToLower(string(out))
	return strings.Contains(s, "builtin\\administrators") || strings.Contains(s, "administrators")
}

// AttemptElevate tries to relaunch the current executable with admin rights using PowerShell.
// Returns (relaunched, error). If relaunched is true, caller should exit.
func AttemptElevate() (bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	// When running with `go run`, this may not behave as expected.
	if strings.HasSuffix(strings.ToLower(exe), "go.exe") {
		return false, errors.New("cannot elevate in go run mode; build the agent first")
	}
	// Re-run self with same arguments
	args := strings.Join(os.Args[1:], ",")
	ps := fmt.Sprintf("Start-Process -FilePath '%s' -ArgumentList '%s' -Verb RunAs", exe, args)
	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	if err := cmd.Start(); err != nil {
		return false, err
	}
	return true, nil
}
