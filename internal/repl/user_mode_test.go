package repl

import "testing"

func TestUserModeTurnPolicy(t *testing.T) {
	tests := []struct {
		mode             UserMode
		wantRoute        TurnRoute
		wantNeedsRepo    bool
		wantNeedsOp      bool
		wantNeedsData    bool
		wantHasPolicy    bool
		wantUsesSelector bool
	}{
		{mode: UserModeAuto, wantHasPolicy: false, wantUsesSelector: true},
		{mode: UserModeCode, wantRoute: RouteRepo, wantNeedsRepo: true, wantHasPolicy: true},
		{mode: UserModeOperation, wantRoute: RouteOperation, wantNeedsOp: true, wantHasPolicy: true},
		{mode: UserModeData, wantRoute: RouteData, wantNeedsData: true, wantHasPolicy: true},
		{mode: UserModeWrite, wantHasPolicy: false},
	}
	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			if got := tt.mode.UsesClassifier(); got != tt.wantUsesSelector {
				t.Fatalf("UsesClassifier() = %v, want %v", got, tt.wantUsesSelector)
			}
			policy, ok := tt.mode.TurnPolicy()
			if ok != tt.wantHasPolicy {
				t.Fatalf("TurnPolicy ok = %v, want %v", ok, tt.wantHasPolicy)
			}
			if !ok {
				return
			}
			if policy.Route != tt.wantRoute {
				t.Fatalf("Route = %q, want %q", policy.Route, tt.wantRoute)
			}
			if policy.NeedsRepoAccess != tt.wantNeedsRepo {
				t.Fatalf("NeedsRepoAccess = %v, want %v", policy.NeedsRepoAccess, tt.wantNeedsRepo)
			}
			if policy.NeedsOperationAccess != tt.wantNeedsOp {
				t.Fatalf("NeedsOperationAccess = %v, want %v", policy.NeedsOperationAccess, tt.wantNeedsOp)
			}
			if policy.NeedsDataAccess != tt.wantNeedsData {
				t.Fatalf("NeedsDataAccess = %v, want %v", policy.NeedsDataAccess, tt.wantNeedsData)
			}
		})
	}
}
