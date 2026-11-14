//go:build !darwin && !linux && !windows

package main

import "errors"

var errNoService = errors.New("cloche setup knows launchd, systemd and Task Scheduler: start cloche serve some other way here")

func installService(string, map[string]string) (service, error) { return service{}, errNoService }

func removeService() (string, error) { return "", errNoService }

func serviceInstalled() bool { return false }
