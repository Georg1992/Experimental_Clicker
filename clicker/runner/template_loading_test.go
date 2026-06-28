package runner

import (
	"testing"
)

// TestTemplateLibraryLoading checks if templates are loaded correctly.
func TestTemplateLibraryLoading(t *testing.T) {
	lib := NewGameFontTemplateLibrary()

	// Check if all digits are loaded
	for ch := '0'; ch <= '9'; ch++ {
		tmpl := lib.templates[ch]
		if tmpl == nil {
			t.Errorf("Template for digit '%c' is nil", ch)
			continue
		}

		if len(tmpl.Pattern) == 0 {
			t.Errorf("Template for digit '%c' has empty pattern", ch)
			continue
		}

		if len(tmpl.Pattern) != 16*24 {
			t.Errorf("Template for digit '%c' has wrong size: %d (expected %d)", ch, len(tmpl.Pattern), 16*24)
			continue
		}

		// Count 1s and 0s
		ones := 0
		zeros := 0
		for _, bit := range tmpl.Pattern {
			if bit == '1' {
				ones++
			} else if bit == '0' {
				zeros++
			}
		}

		t.Logf("Digit '%c': %d bits, %d ones, %d zeros", ch, len(tmpl.Pattern), ones, zeros)
	}

	// Check slash
	tmpl := lib.templates['/']
	if tmpl == nil {
		t.Errorf("Template for '/' is nil")
	} else if len(tmpl.Pattern) != 16*24 {
		t.Errorf("Template for '/' has wrong size: %d", len(tmpl.Pattern))
	} else {
		ones := 0
		for _, bit := range tmpl.Pattern {
			if bit == '1' {
				ones++
			}
		}
		t.Logf("Slash '/': %d bits with %d ones", len(tmpl.Pattern), ones)
	}
}

// TestHammingDistance tests the Hamming distance calculation.
func TestHammingDistance(t *testing.T) {
	pattern1 := "1111111111111111" + "0000000000000000" // 16 bits of 1s, then 16 bits of 0s
	pattern2 := "1111111111111111" + "0000000000000000" // Same

	score := hammingScore(pattern1, pattern2)
	t.Logf("Identical patterns: score=%.3f (should be 1.000)", score)

	if score != 1.0 {
		t.Errorf("Expected 1.0, got %.3f", score)
	}

	// Test with inverse
	pattern3 := "0000000000000000" + "1111111111111111" // Inverse

	score2 := hammingScore(pattern1, pattern3)
	t.Logf("Inverse patterns: score=%.3f (should be 0.000)", score2)

	if score2 != 0.0 {
		t.Errorf("Expected 0.0, got %.3f", score2)
	}

	// Test with 50% match
	pattern4 := "1111111100000000" + "0000000000000000" // 50% match with pattern1

	score3 := hammingScore(pattern1, pattern4)
	t.Logf("50%% match: score=%.3f (should be ~0.625)", score3)
}
