package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
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

func TestTaskQueryReferenceVideoActionIncludesHistoricalRecords(t *testing.T) {
	truncateTables(t)
	insertTask(t, &Task{
		TaskID: "task_reference_video_filter",
		Action: constant.TaskActionTextGenerate,
		Properties: Properties{
			HasReferenceVideo:     true,
			ReferenceVideoSeconds: 7.87,
			VideoGenerationMode:   constant.TaskVideoGenerationModeTextToVideo,
		},
	})
	insertTask(t, &Task{
		TaskID:     "task_text_video_filter",
		Action:     constant.TaskActionTextGenerate,
		Properties: Properties{VideoGenerationMode: constant.TaskVideoGenerationModeTextToVideo},
	})

	tasks := TaskGetAllTasks(0, 10, SyncTaskQueryParams{Action: constant.TaskActionReferenceVideo})

	require.Len(t, tasks, 1)
	assert.Equal(t, "task_reference_video_filter", tasks[0].TaskID)
}
