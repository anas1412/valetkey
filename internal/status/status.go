// Package status builds CRD status blocks with a standard Ready condition.
package status

import (
	"time"

	"github.com/anas1412/llmproxy-k8s/internal/types"
)

// Ready builds a status with a Ready condition. lastTransitionTime is kept from
// prev when readiness did not change.
func Ready(prev types.Status, ready bool, reason, message string, generation int64) types.Status {
	s := "False"
	if ready {
		s = "True"
	}
	ltt := time.Now().UTC().Format(time.RFC3339)
	if old := find(prev); old != nil && old.Status == s {
		ltt = old.LastTransitionTime
	}
	return types.Status{
		Ready:   ready,
		Message: message,
		Conditions: []types.Condition{{
			Type: "Ready", Status: s, Reason: reason, Message: message,
			LastTransitionTime: ltt, ObservedGeneration: generation,
		}},
		KeyHash:    prev.KeyHash,
		SecretName: prev.SecretName,
	}
}

// Changed reports whether next differs from prev in anything worth a status patch.
func Changed(prev, next types.Status) bool {
	p, n := find(prev), find(next)
	if p == nil || n == nil {
		return true
	}
	return p.Reason != n.Reason || p.ObservedGeneration != n.ObservedGeneration ||
		prev.Ready != next.Ready || prev.Message != next.Message ||
		prev.KeyHash != next.KeyHash || prev.SecretName != next.SecretName
}

func find(s types.Status) *types.Condition {
	for i := range s.Conditions {
		if s.Conditions[i].Type == "Ready" {
			return &s.Conditions[i]
		}
	}
	return nil
}
