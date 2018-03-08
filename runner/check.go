package runner

import (
	"context"
	"fmt"
	goexec "os/exec"
	"runtime"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/dcos/dcos-go/exec"
	"github.com/pkg/errors"
)

// signalKilled is an error message returned by dcos-go/exec if the check exceeds timeout
const signalKilled = "signal: killed"

// Check is a basic structure that describes DC/OS check.
type Check struct {
	// Cmd is a path to executable (script or binary) and arguments
	Cmd []string `json:"cmd"`

	// Description provides a basic check description.
	Description string `json:"description"`

	// Timeout is defines a custom script timeout.
	Timeout string `json:"timeout"`

	// Roles is a list of DC/OS roles (e.g. master, agent). Node must be one of roles
	// to execute a check.
	Roles []string `json:"roles"`
}

// Run executes the given check.
func (c *Check) Run(ctx context.Context, role string) ([]byte, int, error) {
	if !c.verifyRole(role) {
		return nil, -1, errors.Errorf("check can be executed on a node with the following roles %s. Current role %s", c.Roles, role)
	}

	if len(c.Cmd) == 0 {
		return nil, -1, errors.New("unable to execute a command with empty Cmd field")
	}

	timeout, err := time.ParseDuration(c.Timeout)
	if err != nil {
		logrus.Warningf("error reading timeout %s. Using default timeout 5sec", err)
		timeout = time.Second * 5
	}

	newCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if runtime.GOOS == "windows" {
		c.Cmd = append([]string{"powershell.exe"}, c.Cmd...)
	}
	stdout, stderr, code, err := exec.FullOutput(exec.CommandContext(newCtx, c.Cmd...))
	if err != nil {
		// check if the error happened due to command timeout and treat it as a failed command
		// instead of error.
		if c.checkTimeout(err) {
			errMsg := fmt.Sprintf("command %s exceeded timeout %s and was killed", c.Cmd, timeout)
			return []byte(errMsg), statusUnknown, nil
		}

		return nil, -1, err
	}

	combinedOutput := append(stdout, stderr...)
	return combinedOutput, code, nil
}

func (c *Check) verifyRole(role string) bool {
	// no roles means we are allowed to execute a check on any node.
	if len(c.Roles) == 0 {
		return true
	}

	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// checkTimeout is a helper function, which takes an error as input parameter and checks if the error
// was due to timeout. This can be validated in the following way. First the error must be an instance of
// exec.ExitError, second the error message must be "signal: killed". Unfortunately go native exec package does not
// expose the concrete error for a process to be killed, that is why we have to type assert first and then check
// against hardcoded string. see https://golang.org/src/os/exec_posix.go Line 85
func (c Check) checkTimeout(e error) bool {
	if exiterr, ok := e.(*goexec.ExitError); ok {
		return signalKilled == exiterr.Error()
	}

	return false
}
