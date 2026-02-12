package sdk

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// generateID generates a random hex ID
func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// currentTimeMs returns current Unix time in milliseconds
func currentTimeMs() int64 {
	return time.Now().UnixMilli()
}
