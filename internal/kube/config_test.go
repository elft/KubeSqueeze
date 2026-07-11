package kube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRESTConfigRequiresAndResolvesExplicitContext(t *testing.T) {
	if _, _, err := RESTConfig(Config{}); err == nil {
		t.Fatal("expected missing context error")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	config := `apiVersion: v1
kind: Config
current-context: wrong
clusters:
- name: chosen-cluster
  cluster:
    server: https://chosen.example.test
- name: wrong-cluster
  cluster:
    server: https://wrong.example.test
contexts:
- name: chosen
  context:
    cluster: chosen-cluster
    user: tester
- name: wrong
  context:
    cluster: wrong-cluster
    user: tester
users:
- name: tester
  user:
    token: test-token
`
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	restConfig, target, err := RESTConfig(Config{Kubeconfig: path, Context: "chosen"})
	if err != nil {
		t.Fatal(err)
	}
	if target.Context != "chosen" || target.Server != "https://chosen.example.test" || restConfig.Host != target.Server {
		t.Fatalf("unexpected resolution: rest=%q target=%+v", restConfig.Host, target)
	}
	_, _, err = RESTConfig(Config{Kubeconfig: path, Context: "missing"})
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected unknown context error, got %v", err)
	}
}
