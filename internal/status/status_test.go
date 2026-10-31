package status

import (
	"testing"

	"github.com/anas1412/llmp/internal/types"
)

func TestReadyKeepsTransitionTimeAndDetectsChanges(t *testing.T) {
	a := Ready(types.Status{}, true, "Active", "Active", 1)
	b := Ready(a, true, "Active", "Active", 1)
	if a.Conditions[0].LastTransitionTime != b.Conditions[0].LastTransitionTime {
		t.Fatal("transition time must be kept when readiness is unchanged")
	}
	if Changed(a, b) {
		t.Fatal("identical status reported as changed")
	}
	if !Changed(a, Ready(a, false, "Disabled", "off", 1)) {
		t.Fatal("readiness change not detected")
	}
	if !Changed(a, Ready(a, true, "Active", "Active", 2)) {
		t.Fatal("generation change not detected")
	}
}
