//go:build darwin || linux

package main

import (
	"os"
	"syscall"
	"unsafe"
)

func terminal(f *os.File) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), getTermios, uintptr(unsafe.Pointer(&t)))
	return errno == 0
}
