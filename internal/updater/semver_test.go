package updater

import "testing"

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"equal", "1.0.0", "1.0.0", 0},
		{"greater major", "2.0.0", "1.0.0", 1},
		{"less major", "1.0.0", "2.0.0", -1},
		{"greater minor", "1.2.0", "1.1.0", 1},
		{"less minor", "1.1.0", "1.2.0", -1},
		{"greater patch", "1.0.2", "1.0.1", 1},
		{"less patch", "1.0.1", "1.0.2", -1},
		{"v prefix both", "v1.0.0", "v1.0.0", 0},
		{"v prefix a only", "v2.0.0", "1.0.0", 1},
		{"partial a", "1.0", "1.0.0", 0},
		{"partial b", "1.0.0", "1.0", 0},
		{"both partial", "1.0", "1.0", 0},
		{"non-numeric", "1.x.0", "1.0.0", 0},
		{"zero is valid", "0.0.0", "0.0.0", 0},
		{"large version", "10.20.30", "9.9.9", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompareSemver(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("CompareSemver(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
