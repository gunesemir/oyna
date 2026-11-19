package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func containerCommand(tool layer, rt, verb string, wrapped []string) ([]string, error) {
	if os.Getenv("OYNA_CONTAINER_NSENTER") != "1" {
		return wrapped, nil
	}
	if rt == "" {
		return nil, fmt.Errorf("container re-entry requires a Steam Linux Runtime")
	}
	container, err := readLayer(rt, verb)
	if err != nil {
		return nil, err
	}
	service := tool.kind
	if service == "umu-passthrough" {
		service = container.kind
	}
	if err = os.Setenv("STEAM_COMPAT_LAUNCHER_SERVICE", service); err != nil {
		return nil, err
	}
	client := filepath.Join(rt, "pressure-vessel", "bin", "steam-runtime-launch-client")
	bus := "com.steampowered.App" + os.Getenv("STEAM_COMPAT_APP_ID")
	for trial := 0; trial < 5; trial++ {
		out, err := exec.Command(client, "--list").Output()
		if err != nil {
			return nil, fmt.Errorf("list runtime containers: %w", err)
		}
		for _, line := range strings.Split(string(out), "\n") {
			if line == "--bus-name="+bus {
				unwrapped, err := readLayer(tool.dir, "runinprefix")
				if err != nil {
					return nil, err
				}
				if err = os.Setenv("PROTON_VERB", "runinprefix"); err != nil {
					return nil, err
				}
				return append([]string{client, "--bus-name=" + bus, "--"}, unwrapped.command...), nil
			}
		}
		if trial < 4 {
			time.Sleep(time.Second)
		}
	}
	// A first launch creates the service, subsequent launches can enter it.
	return wrapped, nil
}
