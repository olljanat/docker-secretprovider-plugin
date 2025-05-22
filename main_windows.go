//go:build windows

package main

import (
	"os"
	"path/filepath"

	"github.com/Freman/eventloghook"
	"github.com/docker/go-plugins-helpers/sdk"
	"github.com/docker/go-plugins-helpers/volume"
	acl "github.com/hectane/go-acl"
	"github.com/olljanat/docker-secretprovider-plugin/hcsshim/security"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
)

var (
	baseDir = filepath.Join(sdk.WindowsDefaultDaemonRootDir(), "secrets")
	npipe   = "//./pipe/docker-secretprovider-plugin"

	// AllowSystemOnly limits pipe access to NT AUTHORITY\SYSTEM
	AllowSystemOnly = "D:(A;;GA;;;SY)"

	ContainerAdministratorSid = "S-1-5-93-2-1"
	ContainerUserSid          = "S-1-5-93-2-2"

	ServiceName = "docker-secret"
)

type program struct {
	h *volume.Handler
}

func (p *program) Execute(args []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	s <- svc.Status{State: svc.StartPending}
	go p.run()
	s <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

loop:
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			s <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			break loop
		}
	}
	s <- svc.Status{State: svc.StopPending}
	return false, 0
}

func (p *program) run() {
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		if err := os.Mkdir(baseDir, os.ModePerm); err != nil {
			log.Fatal(err)
		}
	}

	// TODO: Contribute to github.com/microsoft/hcsshim/tree/main/internal/security
	// so that these would be packages in there.
	if err := acl.Apply(
		baseDir,
		true,
		false,
		acl.GrantName(windows.GENERIC_ALL, "NT AUTHORITY\\SYSTEM"),
	); err != nil {
		panic(err)
	}
	if err := security.GrantVmGroupAccess(baseDir, ContainerAdministratorSid); err != nil {
		log.Fatal(err)
	}
	if err := security.GrantVmGroupAccess(baseDir, ContainerUserSid); err != nil {
		log.Fatal(err)
	}

	config := sdk.WindowsPipeConfig{
		// SecurityDescriptor: sdk.AllowServiceSystemAdmin,
		SecurityDescriptor: AllowSystemOnly,
		InBufferSize:       npipeMaxBuf,
		OutBufferSize:      npipeMaxBuf,
	}
	if err := p.h.ServeWindows(npipe, "secret", sdk.WindowsDefaultDaemonRootDir(), &config); err != nil {
		logrus.Errorf("Error serving volume plugin: %v", err)
	}
}

func serve(h *volume.Handler) {
	prg := &program{h: h}
	if isSvc, err := svc.IsWindowsService(); err == nil && !isSvc {
		log.Infof("Running in interactive mode")
		prg.run()
		return
	}
	err := svc.Run(ServiceName, prg)
	if err != nil {
		log.Fatalf("Failed to start service: %v ", err)
	}
}

func logger() *logrus.Logger {
	log := logrus.New()
	elog, err := eventlog.Open(ServiceName)
	if err != nil {
		panic(err)
	}
	hook := eventloghook.NewHook(*elog)
	log.Hooks.Add(hook)
	return log
}
