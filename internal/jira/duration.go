package jiraclient

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

// durationTokenRegexp matches a numeric amount (optionally fractional) followed
// by a single d/h/m unit. The trailing (.*) captures the remainder of the token
// so compound tokens like "2h30m" are parsed unit by unit.
var durationTokenRegexp = regexp.MustCompile(`^(\d+(?:\.\d+)?)([dhm])(.*)$`)

// NormalizeDuration converts a human-friendly duration string such as "2h30m",
// "1.5h", or "2h 30m" into a canonical "2h 30m" form.
//
// Rules (mirroring the legacy zsh normalize_workload_duration):
//   - Input is split on whitespace; each token is parsed independently.
//   - Supported units: d (days), h (hours), m (minutes).
//   - A fractional amount is only valid for hours; it is converted to minutes
//     (rounded to the nearest minute) and re-expressed as "Xh Ym".
//   - Integer amounts are kept verbatim as "X<unit>".
//   - Normalized tokens are joined with a single space.
func NormalizeDuration(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("empty duration")
	}

	tokens := strings.Fields(input)
	normalized := make([]string, 0, len(tokens))

	for _, token := range tokens {
		rest := token
		for rest != "" {
			m := durationTokenRegexp.FindStringSubmatch(rest)
			if m == nil {
				return "", fmt.Errorf("invalid duration segment %q", rest)
			}
			amount := m[1]
			unit := m[2]
			rest = m[3]

			if strings.Contains(amount, ".") {
				// Fractional amounts are only meaningful for hours.
				if unit != "h" {
					return "", fmt.Errorf("fractional duration only supported for hours, got %q", amount+unit)
				}
				var hours float64
				if _, err := fmt.Sscanf(amount, "%f", &hours); err != nil {
					return "", fmt.Errorf("invalid duration amount %q", amount)
				}
				totalMinutes := int(math.Round(hours * 60))
				h := totalMinutes / 60
				mn := totalMinutes % 60
				if h > 0 {
					normalized = append(normalized, fmt.Sprintf("%dh", h))
				}
				if mn > 0 {
					normalized = append(normalized, fmt.Sprintf("%dm", mn))
				}
			} else {
				normalized = append(normalized, amount+unit)
			}
		}
	}

	if len(normalized) == 0 {
		return "", fmt.Errorf("empty duration")
	}
	return strings.Join(normalized, " "), nil
}
