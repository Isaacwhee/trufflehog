package buffer

import (
	"fmt"
	"sync"

	"github.com/trufflesecurity/trufflehog/v3/pkg/context"
)

type poolMetrics struct{}

func (poolMetrics) recordShrink(amount int) {
	shrinkCount.Inc()
	shrinkAmount.Add(float64(amount))
}

func (poolMetrics) recordBufferRetrival() {
	activeBufferCount.Inc()
	checkoutCount.Inc()
	bufferCount.Inc()
}

func (poolMetrics) recordBufferReturn(bufCap, bufLen int64) {
	activeBufferCount.Dec()
	totalBufferSize.Add(float64(bufCap))
	totalBufferLength.Add(float64(bufLen))
}

// PoolOpts is a function that configures a BufferPool.
type PoolOpts func(pool *Pool)

// Pool of buffers.
type Pool struct {
	*sync.Pool
	bufferSize uint32

	metrics poolMetrics
}

const defaultBufferSize = 1 << 12 // 4KB
// NewBufferPool creates a new instance of BufferPool.
func NewBufferPool(opts ...PoolOpts) *Pool {
	pool := &Pool{bufferSize: defaultBufferSize}

	for _, opt := range opts {
		opt(pool)
	}
	pool.Pool = &sync.Pool{
		New: func() any {
			return NewRingBuffer(int(pool.bufferSize))
		},
	}

	return pool
}

// Get returns a Buffer from the pool.
func (p *Pool) Get(ctx context.Context) *Ring {
	buf, ok := p.Pool.Get().(*Ring)
	if !ok {
		ctx.Logger().Error(fmt.Errorf("Buffer pool returned unexpected type"), "using new Buffer")
		buf = NewRingBuffer(int(p.bufferSize))
	}
	p.metrics.recordBufferRetrival()
	// buf.resetMetric()

	return buf
}

// Put returns a Buffer to the pool.
func (p *Pool) Put(buf *Ring) {
	p.metrics.recordBufferReturn(int64(buf.Cap()), int64(buf.Len()))

	// If the Buffer is more than twice the default size, replace it with a new Buffer.
	// This prevents us from returning very large buffers to the pool.
	const maxAllowedCapacity = 2 * defaultBufferSize
	if buf.Cap() > maxAllowedCapacity {
		p.metrics.recordShrink(buf.Cap() - defaultBufferSize)
		buf = NewRingBuffer(int(p.bufferSize))
	} else {
		buf.Reset()
	}
	// buf.recordMetric()

	p.Pool.Put(buf)
}
