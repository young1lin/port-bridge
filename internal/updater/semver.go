package updater

import (
	"strconv"
	"strings"
)

// CompareSemver compares two semantic version strings (e.g. "1.2.3").
// Returns:
//
//	-1 if a < b
//	 0 if a == b
//	+1 if a > b
func CompareSemver(a, b string) int {
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")

	aParts := strings.SplitN(a, ".", 3)
	bParts := strings.SplitN(b, ".", 3)

	for i := 0; i < 3; i++ {
		av := 0
		bv := 0
		if i < len(aParts) {
			av, _ = strconv.Atoi(aParts[i])
		}
		if i < len(bParts) {
			bv, _ = strconv.Atoi(bParts[i])
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}
