package runtime

import (
	"fmt"
	"sync/atomic"
	"time"
)

var globalRunCounter uint64

func NewRunID(prefix string) string {
	if prefix == "" {
		prefix = "run"
	}
	n := atomic.AddUint64(&globalRunCounter, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), n)
}
