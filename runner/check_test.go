package runner

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestCombinedOutput(t *testing.T) {
	ch1 := &Check{
		Cmd:   []string{CombinedShScript},
		Roles: []string{"master"},
	}

	if runtime.GOOS == "windows" {
		ch1.Cmd = []string{CombinedPowershellScript}
	}

	output, code, err := ch1.Run(context.TODO(), "master")
	if err != nil {
		t.Fatal(err)
	}

	if code != 0 {
		t.Fatalf("expect return code 0. Got %d", code)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "STDOUT") && !strings.Contains(outputStr, "STDERR") {
		t.Fatalf("unexpected output %s", outputStr)
	}
}
