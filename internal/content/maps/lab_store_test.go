package maps

import "testing"

// The store lab is z-fight free (TestNoZFighting covers only the playable maps).
func TestLabStoreNoZFighting(t *testing.T) {
	for _, f := range zFights(LabStore()) {
		t.Error(f)
	}
}
