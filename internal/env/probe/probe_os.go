package probe

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// probeOSFamily returns the OS family string + a human-readable
// version. Family is the discriminator the Recommender keys on
// (apt vs dnf vs brew etc.); version is informational. Failures
// return ("unknown", "") — never errors.
func probeOSFamily() (family, version string) {
	switch runtime.GOOS {
	case "linux":
		return probeLinuxFamily()
	case "darwin":
		return "darwin", probeDarwinVersion()
	case "windows":
		return "windows", probeWindowsVersion()
	case "freebsd", "openbsd", "netbsd":
		return runtime.GOOS, ""
	}
	return "unknown", ""
}

// probeLinuxFamily reads /etc/os-release (universal across all
// modern distros — systemd standard) and maps ID / ID_LIKE to one
// of the canonical families the Recommender knows about.
func probeLinuxFamily() (family, version string) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "linux", ""
	}
	id := ""
	idLike := ""
	prettyName := ""
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"`)
		switch k {
		case "ID":
			id = strings.ToLower(strings.TrimSpace(v))
		case "ID_LIKE":
			idLike = strings.ToLower(strings.TrimSpace(v))
		case "PRETTY_NAME":
			prettyName = strings.TrimSpace(v)
		}
	}
	// Map known distros to families. The "_LIKE" field captures
	// derivatives (Ubuntu → "debian", Rocky → "rhel"); we trust
	// the explicit ID first.
	for _, candidate := range append([]string{id}, strings.Fields(idLike)...) {
		switch candidate {
		case "debian", "ubuntu", "linuxmint", "pop", "elementary", "kali", "raspbian":
			return "debian", prettyName
		case "rhel", "centos", "fedora", "rocky", "almalinux", "ol":
			return "rhel", prettyName
		case "arch", "manjaro", "endeavouros":
			return "arch", prettyName
		case "alpine":
			return "alpine", prettyName
		case "opensuse-leap", "opensuse-tumbleweed", "sles", "opensuse":
			return "suse", prettyName
		case "gentoo":
			return "gentoo", prettyName
		case "void":
			return "void", prettyName
		}
	}
	return "linux", prettyName
}

// probeDarwinVersion reads sw_vers via /usr/libexec/PlistBuddy is
// overkill for one field; just shell out to sw_vers (always
// present on macOS) and parse. Empty on failure.
func probeDarwinVersion() string {
	out, err := captureCmd("sw_vers", "-productVersion")
	if err != nil {
		return ""
	}
	return "macos " + strings.TrimSpace(out)
}

// probeWindowsVersion reads Windows version via cmd.exe / ver.
// Best-effort; PowerShell could give richer info but isn't
// guaranteed to be on PATH.
func probeWindowsVersion() string {
	out, err := captureCmd("cmd", "/c", "ver")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// probeInsideContainer is a heuristic: /.dockerenv exists, or
// /proc/1/cgroup has docker / lxc / kubepods substrings.
// Containers without these markers (rkt, custom runtimes) are
// missed — that's acceptable for an INFO-level fact.
func probeInsideContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		s := string(data)
		for _, marker := range []string{"docker", "lxc", "kubepods", "containerd", "podman"} {
			if strings.Contains(s, marker) {
				return true
			}
		}
	}
	return false
}

// probeShell discovers the operator's shell. SHELL env var is the
// usual oracle; on Windows we check COMSPEC then fall back to
// "unknown".
func probeShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return filepath.Base(s)
	}
	if runtime.GOOS == "windows" {
		if c := os.Getenv("COMSPEC"); c != "" {
			return filepath.Base(c)
		}
	}
	return "unknown"
}
