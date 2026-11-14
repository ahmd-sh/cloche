package main

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	getConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
	freeConsole           = kernel32.NewProc("FreeConsole")
)

func background() {
	var pids [2]uint32
	n, _, _ := getConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	if n != 1 {
		return
	}
	freeConsole.Call()
	if err := os.MkdirAll(home(), 0o700); err != nil {
		return
	}
	path := filepath.Join(home(), "serve.log")
	flags := os.O_CREATE | os.O_APPEND | os.O_WRONLY
	if st, err := os.Stat(path); err == nil && st.Size() > 10<<20 {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return
	}
	os.Stdout, os.Stderr = f, f
}
