package dicom

import (
	"fmt"
	"math/rand"
	"time"
)

// GenerateUID generates a DICOM unique identifier (UID)
// using a prefix and unique components (time, random).
// Format: prefix.<timestamp>.<random>
func GenerateUID(prefix string) string {
	now := time.Now()
	// Simple UID generation strategy
	// 20060102150405 + .nanoseconds + .random
	timestamp := now.Format("20060102150405")
	nano := now.Nanosecond()
	rnd := rand.Intn(10000)

	// Ensure prefix ends with dot
	if len(prefix) > 0 && prefix[len(prefix)-1] != '.' {
		prefix += "."
	}

	return fmt.Sprintf("%s%s.%d.%d", prefix, timestamp, nano, rnd)
}
