package types

import "testing"

func TestAllBugClasses_Coverage(t *testing.T) {
	all := AllBugClasses()
	if len(all) != 19 {
		t.Errorf("AllBugClasses returned %d, expected 19 — adjust if registry expands", len(all))
	}
	seen := make(map[BugClass]bool, len(all))
	for _, c := range all {
		if seen[c] {
			t.Errorf("duplicate BugClass in AllBugClasses: %q", c)
		}
		seen[c] = true
		if !IsValidBugClass(c) {
			t.Errorf("AllBugClasses includes %q but IsValidBugClass returns false", c)
		}
	}
}

func TestIsValidBugClass_Negatives(t *testing.T) {
	for _, bogus := range []BugClass{"", "race_condition", "RACE", "Race", "panic", "unknown"} {
		if IsValidBugClass(bogus) {
			t.Errorf("IsValidBugClass(%q) = true, want false", bogus)
		}
	}
}

func TestDetectedBugClass_HumanLabel(t *testing.T) {
	cases := []struct {
		d    DetectedBugClass
		want string
	}{
		{DetectedBugClass{Class: BugClassRace, HumanZh: "数据竞争", HumanEn: "data race"}, "数据竞争 / data race"},
		{DetectedBugClass{Class: BugClassRace, HumanZh: "数据竞争"}, "数据竞争"},
		{DetectedBugClass{Class: BugClassRace, HumanEn: "data race"}, "data race"},
		{DetectedBugClass{Class: BugClassRace}, "race"}, // fallback to enum
	}
	for _, c := range cases {
		if got := c.d.HumanLabel(); got != c.want {
			t.Errorf("HumanLabel(%+v) = %q, want %q", c.d, got, c.want)
		}
	}
}
