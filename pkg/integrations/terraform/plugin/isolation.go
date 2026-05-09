//go:build linux

package plugin

import (
	"fmt"
	"os/exec"
	"syscall"
)

type EgressMode string

const (
	EgressUnisolated EgressMode = "unisolated"
	EgressDirect     EgressMode = "direct-egress"
	EgressDisabled   EgressMode = "disabled-egress"
	EgressProxy      EgressMode = "proxy-egress"
)

type IsolationOptions struct {
	Egress EgressMode
}

func ApplyIsolation(cmd *exec.Cmd, opts IsolationOptions) error {
	if cmd == nil {
		return fmt.Errorf("command is required")
	}
	egress := opts.Egress
	if egress == "" {
		egress = EgressDirect
	}

	cloneFlags := uintptr(syscall.CLONE_NEWNS | syscall.CLONE_NEWPID)
	switch egress {
	case EgressUnisolated:
		return nil
	case EgressDirect:
	case EgressDisabled:
		cloneFlags |= syscall.CLONE_NEWNET
	case EgressProxy:
		return fmt.Errorf("proxy-egress isolation is reserved and not implemented")
	default:
		return fmt.Errorf("unknown egress mode %q", egress)
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:   cloneFlags,
		Unshareflags: syscall.CLONE_NEWNS,
	}
	return nil
}
