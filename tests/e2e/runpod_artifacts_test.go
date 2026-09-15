package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunPodTemplatesContainNoSecrets(t *testing.T) {
	root := repositoryRoot(t)
	paths := []string{
		filepath.Join(root, "deploy", "runpod", "pod.template.json"),
		filepath.Join(root, "deploy", "runpod", "network-volume.template.json"),
	}

	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var value any
		if err := json.Unmarshal(contents, &value); err != nil {
			t.Errorf("%s is not valid JSON: %v", path, err)
		}
		if strings.Contains(string(contents), "hf_") || strings.Contains(string(contents), "rpa_") {
			t.Errorf("%s appears to contain a real credential", path)
		}
	}
}

func TestPodTemplateUsesExpectedHardware(t *testing.T) {
	path := filepath.Join(repositoryRoot(t), "deploy", "runpod", "pod.template.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var pod struct {
		CloudType     string   `json:"cloudType"`
		GPUCount      int      `json:"gpuCount"`
		GPUTypes      []string `json:"gpuTypeIds"`
		DataCenters   []string `json:"dataCenterIds"`
		NetworkVolume string   `json:"networkVolumeId"`
	}
	if err := json.Unmarshal(contents, &pod); err != nil {
		t.Fatal(err)
	}
	if pod.CloudType != "SECURE" || pod.GPUCount != 1 {
		t.Fatalf("unexpected pod shape: %+v", pod)
	}
	if len(pod.GPUTypes) != 1 || pod.GPUTypes[0] != "NVIDIA H200" {
		t.Fatalf("unexpected GPUs: %v", pod.GPUTypes)
	}
	if len(pod.DataCenters) != 1 || pod.DataCenters[0] != "US-GA-2" {
		t.Fatalf("unexpected datacenters: %v", pod.DataCenters)
	}
	if pod.NetworkVolume != "REPLACE_WITH_NETWORK_VOLUME_ID" {
		t.Fatal("network volume placeholder must be explicit")
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

