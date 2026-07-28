package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAsyncTaskPollHandlerUsesFiveSecondInterval(t *testing.T) {
	assert.Equal(t, 5*time.Second, (asyncTaskPollHandler{}).Interval())
}
