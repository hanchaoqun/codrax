package hitraceconv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// RuntimeAnchorFallback returns the home-directory runtime anchor on WSL when
// it differs from primary. This only supplies a fallback candidate: conversion
// still prefers primary and requires either root to enforce private staging.
// No fallback is offered on native Linux or other operating systems.
func RuntimeAnchorFallback(primary string) string {
	if runtime.GOOS != "linux" {
		return ""
	}
	release, _ := os.ReadFile("/proc/sys/kernel/osrelease")
	return RuntimeAnchorFallbackFor(
		runtime.GOOS,
		string(release),
		os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "",
		primary,
	)
}

// RuntimeAnchorFallbackFor applies the same policy to explicit host facts.
// Product wrappers may use it to preserve their platform-specific contracts;
// ordinary callers should use RuntimeAnchorFallback to detect the current host.
func RuntimeAnchorFallbackFor(goos, kernelRelease string, wslEnvironment bool, primary string) string {
	return runtimeAnchorFallbackFor(goos, kernelRelease, wslEnvironment, primary, os.UserHomeDir)
}

func runtimeAnchorFallbackFor(goos, kernelRelease string, wslEnvironment bool, primary string, userHomeDir func() (string, error)) string {
	release := strings.ToLower(strings.TrimSpace(kernelRelease))
	if goos != "linux" || (!wslEnvironment && !strings.Contains(release, "microsoft") && !strings.Contains(release, "wsl")) {
		return ""
	}
	userHome, err := userHomeDir()
	if err != nil || strings.TrimSpace(userHome) == "" {
		return ""
	}
	fallback, err := filepath.Abs(filepath.Join(userHome, ".codrax"))
	if err != nil {
		return ""
	}
	primaryAbs, err := filepath.Abs(filepath.Clean(primary))
	if err == nil && primaryAbs == fallback {
		return ""
	}
	return fallback
}
