//go:build windows

package main

import (
	"os"
	"path/filepath"

	"github.com/docker/go-plugins-helpers/sdk"
	"github.com/docker/go-plugins-helpers/volume"
	acl "github.com/hectane/go-acl"
	"github.com/olljanat/docker-secretprovider-plugin/hcsshim/security"
	"golang.org/x/sys/windows"
)

var (
	baseDir = filepath.Join(sdk.WindowsDefaultDaemonRootDir(), "secrets")
	npipe   = "//./pipe/docker-secretprovider-plugin"

	// AllowSystemOnly limits access to named pipe for NT AUTHORITY\SYSTEM only
	AllowSystemOnly = "D:(A;;GA;;;SY)"

	ContainerAdministratorSid = "S-1-5-93-2-1"
	ContainerUserSid          = "S-1-5-93-2-2"
)

func serve(h *volume.Handler) {
	log.Infof("Creating secrets folder: %v", baseDir)
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		if err := os.Mkdir(baseDir, os.ModePerm); err != nil {
			log.Fatal(err)
		}
	}

	// TODO: Contribute to github.com/microsoft/hcsshim/tree/main/internal/security
	// so that these would be packages in there.
	log.Infof("Limit secrets folder to NT AUTHORITY\\SYSTEM only")
	if err := acl.Apply(
		baseDir,
		true,
		false,
		acl.GrantName(windows.GENERIC_ALL, "NT AUTHORITY\\SYSTEM"),
	); err != nil {
		panic(err)
	}
	log.Infof("Grant permissions for ContainerAdministrator")
	if err := security.GrantVmGroupAccess(baseDir, ContainerAdministratorSid); err != nil {
		log.Fatal(err)
	}
	log.Infof("Grant permissions for ContainerUser")
	if err := security.GrantVmGroupAccess(baseDir, ContainerUserSid); err != nil {
		log.Fatal(err)
	}

	config := sdk.WindowsPipeConfig{
		// SecurityDescriptor: sdk.AllowServiceSystemAdmin,
		SecurityDescriptor: AllowSystemOnly,
		InBufferSize:       4096,
		OutBufferSize:      4096,
	}
	if err := h.ServeWindows(npipe, "secret", sdk.WindowsDefaultDaemonRootDir(), &config); err != nil {
		log.Errorf("Error serving volume plugin: %v", err)
	}
}
