package kling

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTaskResultMapsUnknownStatusToZeroProgress(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"data":{"task_status":"new_provider_status"}}`))

	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusUnknown, result.Status)
	assert.Equal(t, "0%", result.Progress)
}
