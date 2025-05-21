//go:build linux

package main

import (
	"github.com/docker/go-plugins-helpers/volume"
	"github.com/sirupsen/logrus"
)

var (
	baseDir = "/secrets"
)

func serve(h *volume.Handler) {
	if err := h.ServeUnix("secret", 0); err != nil {
		log.Errorf("Error serving volume plugin: %v", err)
	}
}

func logger() *logrus.Logger {
	return logrus.New()
}
