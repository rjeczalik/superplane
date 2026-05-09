//go:build !linux

package plugin

import (
	"fmt"
	"log"
	"os/exec"
	"sync"
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

var isolationStubWarning sync.Once

func ApplyIsolation(cmd *exec.Cmd, opts IsolationOptions) error {
	if cmd == nil {
		return fmt.Errorf("command is required")
	}
	egress := opts.Egress
	if egress == "" {
		egress = EgressDirect
	}
	switch egress {
	case EgressUnisolated:
		return nil
	case EgressDirect, EgressDisabled:
	case EgressProxy:
		return fmt.Errorf("proxy-egress isolation is reserved and not implemented")
	default:
		return fmt.Errorf("unknown egress mode %q", egress)
	}

	isolationStubWarning.Do(func() {
		log.Printf("terraform provider process isolation is not available on this platform")
	})
	return nil
}
