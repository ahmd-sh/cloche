package main

func private() (restore func()) { return func() {} }
