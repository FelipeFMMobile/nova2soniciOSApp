package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestIAMPolicyIsLeastPrivilege(t *testing.T) {
	path := filepath.Join(repositoryRoot(t), "deploy", "aws", "iam-policy.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var policy struct {
		Statement []struct {
			Action   string `json:"Action"`
			Resource string `json:"Resource"`
		} `json:"Statement"`
	}
	if err := json.Unmarshal(contents, &policy); err != nil {
		t.Fatal(err)
	}
	if len(policy.Statement) != 1 {
		t.Fatalf("expected one statement, got %d", len(policy.Statement))
	}
	statement := policy.Statement[0]
	if statement.Action != "bedrock:InvokeModelWithBidirectionalStream" {
		t.Fatalf("unexpected action %q", statement.Action)
	}
	if statement.Resource != "arn:aws:bedrock:us-east-1::foundation-model/amazon.nova-2-sonic-v1:0" {
		t.Fatalf("unexpected resource %q", statement.Resource)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
