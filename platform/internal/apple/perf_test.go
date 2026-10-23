//go:build darwin && !ios

package apple

import "testing"

func TestProcessMemory(t *testing.T) {
	memory, ok := ProcessMemory()
	if !ok || memory == 0 {
		t.Fatalf("ProcessMemory() = %d, %v", memory, ok)
	}
}

func TestGPUProbeSamples(t *testing.T) {
	probe := NewGPUProbe()
	if probe == nil {
		t.Skip("no accelerator")
	}
	// The first sample establishes the baseline; a machine with an
	// accelerator must report a reading or none, never panic.
	_ = probe.Sample()
}
