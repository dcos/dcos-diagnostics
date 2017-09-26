package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/pkg/errors"
)

func TestConfigLoadConfig(t *testing.T) {
	c := &Runner{}
	if err := c.LoadFromFile("./fixture/checks.json"); err != nil {
		t.Fatal(err)
	}
}

func TestNewRunner(t *testing.T) {
	r := NewRunner("master")
	if r.role != "master" {
		t.Fatalf("expecting role master. Got %s", r.role)
	}

	r = NewRunner("agent")
	if r.role != "agent" {
		t.Fatalf("expecting role agent. Got %s", r.role)
	}

	r = NewRunner("agent_public")
	if r.role != "agent" {
		t.Fatalf("expecting role agent. Got %s", r.role)
	}
}

func TestRun(t *testing.T) {
	r := NewRunner("master")
	cfg := `
{
  "cluster_checks": {
    "test_check": {
      "cmd": ["./fixture/combined.sh"],
      "timeout": "1s"
    }
  },
  "node_checks": {
    "checks": {
      "check1": {
        "cmd": ["./fixture/combined.sh"],
        "timeout": "1s"
      },
      "check2": {
        "cmd": ["./fixture/combined.sh"],
        "timeout": "1s"
      }
    },
    "prestart": ["check1"],
    "poststart": ["check2"]
  }
}`
	r.Load(strings.NewReader(cfg))
	out, err := r.Cluster(context.TODO(), false)
	if err != nil {
		t.Fatal(err)
	}

	expectedOutput := "STDOUT\nSTDERR\n"

	if err := validateCheck(out, "test_check", expectedOutput); err != nil {
		t.Fatal(err)
	}

	prestart, err := r.PreStart(context.TODO(), false)
	if err != nil {
		t.Fatal(err)
	}

	if err := validateCheck(prestart, "check1", expectedOutput); err != nil {
		t.Fatal(err)
	}

	poststart, err := r.PostStart(context.TODO(), false)
	if err != nil {
		t.Fatal(err)
	}

	if err := validateCheck(poststart, "check2", expectedOutput); err != nil {
		t.Fatal(err)
	}
}

func validateCheck(cr *CombinedResponse, name, output string) error {
	if cr.Status() != 0 {
		return errors.Errorf("expect exit code 0. Got %d", cr.Status())
	}

	check, ok := cr.checks[name]
	if !ok {
		return errors.Errorf("expect check %s", name)
	}

	if check.output != output {
		return errors.Errorf("expect %s. Got %s", output, check.output)
	}
	return nil
}

func TestList(t *testing.T) {
	r := NewRunner("master")
	cfg := `
{
  "cluster_checks": {
    "cluster_check_1": {
      "description": "Cluster check 1",
      "cmd": ["echo", "cluster_check_1"],
      "timeout": "1s"
    }
  },
  "node_checks": {
    "checks": {
      "node_check_1": {
        "description": "Node check 1",
        "cmd": ["echo", "node_check_1"],
        "timeout": "1s"
      },
      "node_check_2": {
        "description": "Node check 2",
        "cmd": ["echo", "node_check_2"],
        "timeout": "1s",
        "roles": ["master"]
      },
      "node_check_3": {
        "description": "Node check 3",
        "cmd": ["echo", "node_check_3"],
        "timeout": "1s",
        "roles": ["agent"]
      }
    },
    "prestart": ["node_check_1"],
    "poststart": ["node_check_2", "node_check_3"]
  }
}`
	r.Load(strings.NewReader(cfg))

	out, err := r.Cluster(context.TODO(), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCheckListing(out, "cluster_check_1", "Cluster check 1", "1s", []string{"echo", "cluster_check_1"}); err != nil {
		t.Fatal(err)
	}

	out, err = r.PreStart(context.TODO(), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCheckListing(out, "node_check_1", "Node check 1", "1s", []string{"echo", "node_check_1"}); err != nil {
		t.Fatal(err)
	}

	out, err = r.PostStart(context.TODO(), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCheckListing(out, "node_check_2", "Node check 2", "1s", []string{"echo", "node_check_2"}); err != nil {
		t.Fatal(err)
	}

	// This runner is for a master, so a check that only runs on agents should not be listed.
	unexpectedCheckName := "node_check_3"
	if _, ok := out.checks[unexpectedCheckName]; ok {
		t.Fatalf("found unexpected check %s", unexpectedCheckName)
	}
}

func validateCheckListing(cr *CombinedResponse, name, description, timeout string, cmd []string) error {
	check, ok := cr.checks[name]
	if !ok {
		return errors.Errorf("expect check %s", name)
	}

	if check.description != description {
		return errors.Errorf("expect description %s. Got %s", description, check.description)
	}

	if check.timeout != timeout {
		return errors.Errorf("expect timeout %s. Got %s", timeout, check.timeout)
	}

	for i := range check.cmd {
		if check.cmd[i] != cmd[i] {
			return errors.Errorf("expect cmd %s. Got %s", cmd, check.cmd)
		}
	}

	return nil
}
