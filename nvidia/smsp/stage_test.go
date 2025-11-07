package smsp

import "testing"

// Test that an unseen opcode similar to an existing one returns the existing
// pipeline when similarity >= SimilarityThreshold.
func TestGetPipelineStages_SimilarMatch(t *testing.T) {
	// make threshold permissive for this test
	SimilarityThreshold = 0.50

	// "FFMA.RM" exists in PipelineTable; create a near miss.
	unseen := "FFMA.RMX"

	tpl := getPipelineStages(unseen)
	if tpl.Opcode != "FFMA.RM" {
		t.Fatalf("expected similar opcode 'FFMA.RM', got '%s' (threshold=%.2f)", tpl.Opcode, SimilarityThreshold)
	}
}

// Test that an unknown opcode with no similar entry falls back to default.
func TestGetPipelineStages_FallbackDefault(t *testing.T) {
	// make threshold very high to force fallback
	SimilarityThreshold = 0.99

	unseen := "THIS_OPCODE_DOES_NOT_EXIST_AT_ALL"

	tpl := getPipelineStages(unseen)
	if tpl.Opcode != unseen {
		t.Fatalf("expected defaultStages for '%s', got template opcode '%s'", unseen, tpl.Opcode)
	}
}
