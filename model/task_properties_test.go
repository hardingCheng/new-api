package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskPropertiesReferenceVideoSecondsPreservesDecimalPrecision(t *testing.T) {
	properties := Properties{ReferenceVideoSeconds: 4.126}
	value, err := properties.Value()
	require.NoError(t, err)

	data, ok := value.([]byte)
	require.True(t, ok)

	var restored Properties
	require.NoError(t, restored.Scan(data))
	assert.InDelta(t, 4.126, restored.ReferenceVideoSeconds, 0.0001)
}

func TestTaskPropertiesReferenceVideoSecondsReadsHistoricalInteger(t *testing.T) {
	var properties Properties
	require.NoError(t, properties.Scan([]byte(`{"reference_video_seconds":4}`)))
	assert.Equal(t, 4.0, properties.ReferenceVideoSeconds)
}
