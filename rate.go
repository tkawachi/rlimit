package rlimit

import (
	"time"
)

// Rate configuration
type Rate struct {
	Count    uint32
	Duration time.Duration
}
