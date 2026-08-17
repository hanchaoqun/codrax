package hitraceconv

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelectSecureConversionRuntimeAnchorFallbackIsSecurityOnly(t *testing.T) {
	primary := filepath.Join(t.TempDir(), "primary")
	fallback := filepath.Join(t.TempDir(), "fallback")
	securityErr := errors.Join(errPrivateConversionDirSecurityInvalid, errors.New("mode=0777, want 0700"))

	t.Run("primary secure", func(t *testing.T) {
		var calls []string
		got, err := selectSecureConversionRuntimeAnchorWithProbe(primary, fallback, func(root string) error {
			calls = append(calls, root)
			return nil
		})
		if err != nil || got != primary || len(calls) != 1 || calls[0] != primary {
			t.Fatalf("secure primary selection got=%q err=%v calls=%v", got, err, calls)
		}
	})

	t.Run("security incapable primary", func(t *testing.T) {
		var calls []string
		got, err := selectSecureConversionRuntimeAnchorWithProbe(primary, fallback, func(root string) error {
			calls = append(calls, root)
			if root == primary {
				return securityErr
			}
			return nil
		})
		if err != nil || got != fallback || len(calls) != 2 || calls[0] != primary || calls[1] != fallback {
			t.Fatalf("security fallback selection got=%q err=%v calls=%v", got, err, calls)
		}
	})

	t.Run("ordinary failure remains loud", func(t *testing.T) {
		var calls []string
		got, err := selectSecureConversionRuntimeAnchorWithProbe(primary, fallback, func(root string) error {
			calls = append(calls, root)
			return errors.New("permission denied")
		})
		if got != "" || err == nil || len(calls) != 1 || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("ordinary failure was hidden got=%q err=%v calls=%v", got, err, calls)
		}
	})

	t.Run("fallback remains equally strict", func(t *testing.T) {
		got, err := selectSecureConversionRuntimeAnchorWithProbe(primary, fallback, func(root string) error {
			if root == primary {
				return securityErr
			}
			return errors.Join(errPrivateConversionDirSecurityInvalid, errors.New("fallback mode=0777"))
		})
		if got != "" || err == nil || !strings.Contains(err.Error(), "fallback cannot enforce") {
			t.Fatalf("insecure fallback was accepted got=%q err=%v", got, err)
		}
	})
}

func TestTraceStreamerPrivateDirPatternKeepsCustomerShapeBelowLegacyWindowsLimit(t *testing.T) {
	const customerLeaf = "DH_DT_MLN-L29_target_7ZJ0226530000020_20260811185525_A_20260811185646026_7ZJ0226530000020_PerformanceDynamic_DHweixin_0080_143.4.1.62_hitrace_record_trace_20260812035041@32095-930824219.sys"
	privateLeaf := strings.Replace(traceStreamerPrivateDirPattern, "*", strings.Repeat("0", 32), 1)
	path := `D:\temp\微信启动\.codrax\` + privateLeaf + `\` + customerLeaf
	if len([]rune(path)) >= 260 {
		t.Fatalf("customer-shape trace_streamer argv remains outside the legacy Windows path budget: chars=%d path=%s", len([]rune(path)), path)
	}
	if len(privateLeaf) != len("ts-")+32 {
		t.Fatalf("private directory lost the full 128-bit random suffix: %q", privateLeaf)
	}
}
