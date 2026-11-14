//go:build unix

package main

import "syscall"

func private() (restore func()) {
	old := syscall.Umask(0o077)
	return func() { syscall.Umask(old) }
}
