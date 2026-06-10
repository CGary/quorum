package observability

import (
	"fmt"
	"sync/atomic"
	"time"
)

var seq uint64

func nextID(prefix string) string {
	n := atomic.AddUint64(&seq, 1)
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UTC().UnixNano(), n)
}
