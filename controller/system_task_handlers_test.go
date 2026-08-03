package controller

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsyncTaskPollHandlerUsesStableRandomOneToThreeSecondInterval(t *testing.T) {
	handler := asyncTaskPollHandler{}
	assert.Equal(t, time.Second, handler.Interval())

	seen := map[time.Duration]bool{}
	for i := range 100 {
		latest := &model.SystemTask{TaskID: fmt.Sprintf("systask_%d", i)}
		interval := handler.IntervalAfterLatest(latest)
		require.GreaterOrEqual(t, interval, time.Second)
		require.LessOrEqual(t, interval, 3*time.Second)
		assert.Equal(t, interval, handler.IntervalAfterLatest(latest))
		seen[interval] = true
	}

	assert.Equal(t, map[time.Duration]bool{
		time.Second:     true,
		2 * time.Second: true,
		3 * time.Second: true,
	}, seen)
}
