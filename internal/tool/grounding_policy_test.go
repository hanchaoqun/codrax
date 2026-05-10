package tool

import "testing"

func TestGroundingPolicyForProfile(t *testing.T) {
	cases := []struct {
		name          string
		wantProfile   GroundingProfile
		wantGrounding float64
		wantTier1     float64
		wantOK        bool
	}{
		{"permissive", GroundingProfilePermissive, 0, 0, true},
		{"balanced", GroundingProfileBalanced, 0.5, 0.3, true},
		{"strict", GroundingProfileStrict, 0.8, 0.6, true},
		{"custom", GroundingProfileCustom, 0, 0, true},
		{"unknown", GroundingProfilePermissive, 0, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := GroundingPolicyForProfile(c.name)
			if ok != c.wantOK {
				t.Fatalf("ok=%v, want %v", ok, c.wantOK)
			}
			if got.Profile != c.wantProfile {
				t.Fatalf("profile=%q, want %q", got.Profile, c.wantProfile)
			}
			if got.GroundingFloor != c.wantGrounding {
				t.Fatalf("GroundingFloor=%v, want %v", got.GroundingFloor, c.wantGrounding)
			}
			if got.Tier1Floor != c.wantTier1 {
				t.Fatalf("Tier1Floor=%v, want %v", got.Tier1Floor, c.wantTier1)
			}
		})
	}
}

func TestGroundingPolicyManualFloorsSwitchToCustom(t *testing.T) {
	p := DefaultGroundingPolicy().WithGroundingFloor(0.7).WithTier1Floor(0.4)
	if p.Profile != GroundingProfileCustom {
		t.Fatalf("profile=%q, want custom", p.Profile)
	}
	if p.GroundingFloor != 0.7 {
		t.Fatalf("GroundingFloor=%v, want 0.7", p.GroundingFloor)
	}
	if p.Tier1Floor != 0.4 {
		t.Fatalf("Tier1Floor=%v, want 0.4", p.Tier1Floor)
	}
	if p.WarnGroundingFloor < p.GroundingFloor {
		t.Fatalf("WarnGroundingFloor=%v below hard floor %v", p.WarnGroundingFloor, p.GroundingFloor)
	}
	if p.WarnTier1Floor < p.Tier1Floor {
		t.Fatalf("WarnTier1Floor=%v below hard floor %v", p.WarnTier1Floor, p.Tier1Floor)
	}
}
