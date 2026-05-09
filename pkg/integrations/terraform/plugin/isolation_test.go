package plugin

import (
	"os/exec"
	"runtime"
	"testing"
)

func TestApplyIsolation(t *testing.T) {
	cmd := exec.Command("echo", "ok")
	if err := ApplyIsolation(cmd, IsolationOptions{}); err != nil {
		t.Fatalf("ApplyIsolation() error = %v", err)
	}
	if runtime.GOOS == "linux" && cmd.SysProcAttr == nil {
		t.Fatal("linux ApplyIsolation did not set SysProcAttr")
	}
	if runtime.GOOS != "linux" && cmd.SysProcAttr != nil {
		t.Fatal("non-linux ApplyIsolation should leave SysProcAttr nil")
	}
}

func TestApplyIsolationCanBeDisabled(t *testing.T) {
	cmd := exec.Command("echo", "ok")
	if err := ApplyIsolation(cmd, IsolationOptions{Egress: EgressUnisolated}); err != nil {
		t.Fatalf("ApplyIsolation() error = %v", err)
	}
	if cmd.SysProcAttr != nil {
		t.Fatal("disabled isolation should not set SysProcAttr")
	}
}

func TestApplyIsolationRejectsUnsupportedModes(t *testing.T) {
	if err := ApplyIsolation(exec.Command("echo"), IsolationOptions{Egress: EgressProxy}); err == nil {
		t.Fatal("expected proxy-egress error")
	}
	if err := ApplyIsolation(exec.Command("echo"), IsolationOptions{Egress: "bad"}); err == nil {
		t.Fatal("expected unknown egress error")
	}
	if err := ApplyIsolation(nil, IsolationOptions{}); err == nil {
		t.Fatal("expected nil command error")
	}
}
