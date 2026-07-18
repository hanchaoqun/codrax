//go:build !unix && !windows

package hitraceconv

func externalToolSupervisorTestIgnoreGracefulSignal() {}
func externalToolSupervisorTestProcessAlive(int) bool { return false }
