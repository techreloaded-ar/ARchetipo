package execution

import "testing"

func TestOrphanedTellsACarriedRunFromALostOne(t *testing.T) {
	const self, other = 100, 200
	alive := func(pid int) bool { return pid == self || pid == other }
	cases := []struct {
		name   string
		record Execution
		want   bool
	}{
		{"a terminal record is never an orphan", Execution{Status: StatusFailed, OwnerPID: 999}, false},
		{"a record written before owners existed", Execution{Status: StatusRunning}, true},
		{"a record this process is carrying", Execution{Status: StatusRunning, OwnerPID: self}, false},
		{"a record another live process is carrying", Execution{Status: StatusRunning, OwnerPID: other}, false},
		{"a record whose carrier is gone", Execution{Status: StatusRunning, OwnerPID: 999}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Orphaned(tc.record, self, alive); got != tc.want {
				t.Fatalf("Orphaned = %v, want %v", got, tc.want)
			}
		})
	}
}
