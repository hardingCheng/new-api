package hailuo

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTaskResultMapsUnknownStatusToZeroProgress(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"status":"new_provider_status","base_resp":{"status_code":0}}`))

	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusUnknown, result.Status)
	assert.Equal(t, "0%", result.Progress)
}
