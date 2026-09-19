package hitraceconv

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRuntimeAnchorFallbackForPlatformAndSignals(t *testing.T) {
	userHome := t.TempDir()
	primary := filepath.Join(t.TempDir(), ".codrax")
	want := filepath.Join(userHome, ".codrax")
	for _, test := range []struct {
		name        string
		goos        string
		release     string
		environment bool
		want        string
	}{
		{name: "Microsoft kernel", goos: "linux", release: "5.15.167.4-microsoft-standard-WSL2", want: want},
		{name: "WSL kernel", goos: "linux", release: "6.6.0-WSL2", want: want},
		{name: "normalized kernel", goos: "linux", release: " \tMICROSOFT-STANDARD\n", want: want},
		{name: "environment signal", goos: "linux", release: "6.8.0-generic", environment: true, want: want},
		{name: "unavailable kernel with environment signal", goos: "linux", environment: true, want: want},
		{name: "ordinary Linux", goos: "linux", release: "6.8.0-generic"},
		{name: "unavailable kernel without environment signal", goos: "linux"},
		{name: "Windows host", goos: "windows", release: "microsoft-standard-WSL2", environment: true},
		{name: "Darwin host", goos: "darwin", release: "microsoft-standard-WSL2", environment: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			homeCalls := 0
			got := runtimeAnchorFallbackFor(test.goos, test.release, test.environment, primary, func() (string, error) {
				homeCalls++
				return userHome, nil
			})
			if got != test.want {
				t.Fatalf("fallback=%q want=%q", got, test.want)
			}
			wantCalls := 0
			if test.want != "" {
				wantCalls = 1
			}
			if homeCalls != wantCalls {
				t.Fatalf("home lookup calls=%d want=%d", homeCalls, wantCalls)
			}
		})
	}
	if _, err := os.Stat(want); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fallback selection created or changed its candidate: %v", err)
	}
}

func TestRuntimeAnchorFallbackForHomeAndPrimary(t *testing.T) {
	userHome := t.TempDir()
	primary := filepath.Join(t.TempDir(), ".codrax")
	for _, test := range []struct {
		name    string
		primary string
		home    string
		err     error
		want    string
	}{
		{name: "home unavailable", primary: primary, err: errors.New("home unavailable")},
		{name: "empty home", primary: primary},
		{name: "blank home", primary: primary, home: " \t"},
		{name: "same anchor", primary: filepath.Join(userHome, ".codrax"), home: userHome},
		{name: "same anchor after cleaning", primary: userHome + string(filepath.Separator) + "other" + string(filepath.Separator) + ".." + string(filepath.Separator) + ".codrax", home: userHome},
		{name: "distinct anchor", primary: primary, home: userHome, want: filepath.Join(userHome, ".codrax")},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := runtimeAnchorFallbackFor("linux", "microsoft-standard-WSL2", false, test.primary, func() (string, error) {
				return test.home, test.err
			})
			if got != test.want {
				t.Fatalf("fallback=%q want=%q", got, test.want)
			}
		})
	}
}

func TestRuntimeAnchorFallbackCurrentHostEnvironmentSignals(t *testing.T) {
	primary := filepath.Join(t.TempDir(), ".codrax")
	for _, signal := range []string{"WSL_DISTRO_NAME", "WSL_INTEROP"} {
		t.Run(signal, func(t *testing.T) {
			t.Setenv("WSL_DISTRO_NAME", "")
			t.Setenv("WSL_INTEROP", "")
			t.Setenv(signal, "test-wsl-signal")
			want := RuntimeAnchorFallbackFor(runtime.GOOS, "", true, primary)
			if got := RuntimeAnchorFallback(primary); got != want {
				t.Fatalf("current-host fallback=%q want=%q", got, want)
			}
		})
	}
}
