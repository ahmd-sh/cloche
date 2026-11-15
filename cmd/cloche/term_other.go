//go:build !darwin && !linux

package main

import "os"

func terminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
