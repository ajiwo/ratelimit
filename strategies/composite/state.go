package composite

import (
	"strings"

	"github.com/ajiwo/ratelimit/utils/builderpool"
)

// encodeState creates a composite state encoding from primary and secondary states
// Format: 51|primaryState$secondaryState
func encodeState(primaryState, secondaryState string) string {
	sb := builderpool.Get()
	defer builderpool.Put(sb)

	sb.WriteString("51|")
	sb.WriteString(primaryState)
	sb.WriteString("$")
	sb.WriteString(secondaryState)
	return sb.String()
}

// decodeState extracts primary and secondary states from composite encoding
// Returns empty strings for both if decoding fails
func decodeState(compositeState string) (primaryState, secondaryState string) {
	if len(compositeState) < 3 || compositeState[:3] != "51|" {
		return "", ""
	}

	content := compositeState[3:] // Skip "51|"
	parts := strings.SplitN(content, "$", 2)
	if len(parts) != 2 {
		return "", ""
	}

	return parts[0], parts[1]
}
