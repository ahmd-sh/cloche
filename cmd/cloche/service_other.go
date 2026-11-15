//go:build !darwin && !linux

package main

import "errors"

var errNoService = errors.New("cloche setup knows launchd and systemd: start cloche serve some other way here")

func installService(string, map[string]string) (service, error) { return service{}, errNoService }

func stopService() {}

func removeService() (string, error) { return "", errNoService }

func serviceInstalled() bool { return false }
