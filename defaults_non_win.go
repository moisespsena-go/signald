// +build !windows

package signald

import "syscall"

func init() {
	RestartSignal = syscall.SIGUSR2
}
