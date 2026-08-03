package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTasksToDtoRecursivelyRedactsUserTaskData(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public",
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-private-task-id",
		},
		Properties: model.Properties{
			OriginModelName:   "public-video-model",
			UpstreamModelName: "provider-secret-model",
		},
		Data: []byte(`{
			"id":"upstream-id",
			"task_id":"upstream-task-id",
			"taskId":"upstream-camel-task-id",
			"operationName":"projects/provider-project/models/provider-secret-model/operations/upstream-operation",
			"model":"provider-secret-model",
			"modelName":"provider-secret-camel-model",
			"usage":{"cost":123},
			"usageMetadata":{"tokens":456},
			"response":{
				"id":"nested-upstream-id",
				"task_id":"nested-upstream-task-id",
				"external_task_id":"nested-external-task-id",
				"model":"nested-provider-model",
				"upstreamModelName":"nested-provider-secret-model",
				"status":"processing",
				"operation":{
					"name":"projects/provider-project/models/nested-provider-model/operations/nested-upstream-operation"
				}
			},
			"camelResponse":{
				"id":"camel-upstream-id",
				"taskId":"camel-upstream-task-id",
				"modelName":"camel-provider-model",
				"status":"processing"
			},
			"batches":[[{
				"taskID":"array-upstream-task-id",
				"model_name":"array-provider-model",
				"usage_metadata":{"tokens":789},
				"status":"processing"
			}]],
			"result":{"id":"asset-id","name":"final-video.mp4"}
		}`),
	}

	items := tasksToDto([]*model.Task{task}, taskDtoOptions{})
	require.Len(t, items, 1)
	assert.Empty(t, items[0].UpstreamTaskID)

	properties, ok := items[0].Properties.(model.Properties)
	require.True(t, ok)
	assert.Empty(t, properties.UpstreamModelName)
	assert.Equal(t, "public-video-model", properties.OriginModelName)

	var data map[string]any
	require.NoError(t, common.Unmarshal(items[0].Data, &data))
	assert.Equal(t, "task_public", data["id"])
	assert.Equal(t, "task_public", data["task_id"])
	assert.Equal(t, "task_public", data["taskId"])
	assert.Equal(t, "task_public", data["operationName"])
	assert.Equal(t, "public-video-model", data["model"])
	assert.Equal(t, "public-video-model", data["modelName"])
	assert.NotContains(t, data, "usage")
	assert.NotContains(t, data, "usageMetadata")

	response, ok := data["response"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "task_public", response["id"])
	assert.Equal(t, "task_public", response["task_id"])
	assert.Equal(t, "task_public", response["external_task_id"])
	assert.Equal(t, "public-video-model", response["model"])
	assert.NotContains(t, response, "upstreamModelName")
	operation, ok := response["operation"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "task_public", operation["name"])
	camelResponse, ok := data["camelResponse"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "task_public", camelResponse["id"])
	assert.Equal(t, "task_public", camelResponse["taskId"])
	assert.Equal(t, "public-video-model", camelResponse["modelName"])
	batches, ok := data["batches"].([]any)
	require.True(t, ok)
	require.Len(t, batches, 1)
	batch, ok := batches[0].([]any)
	require.True(t, ok)
	require.Len(t, batch, 1)
	batchTask, ok := batch[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "task_public", batchTask["taskID"])
	assert.Equal(t, "public-video-model", batchTask["model_name"])
	assert.NotContains(t, batchTask, "usage_metadata")

	result, ok := data["result"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "asset-id", result["id"])
	assert.Equal(t, "final-video.mp4", result["name"])
	assert.NotContains(t, string(items[0].Data), "provider-project")
	assert.NotContains(t, string(items[0].Data), "provider-secret-model")
	assert.NotContains(t, string(items[0].Data), "upstream-operation")
}

func TestTasksToDtoIncludesUpstreamTaskIDForAdmin(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public",
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-private-task-id",
		},
	}

	items := tasksToDto([]*model.Task{task}, taskDtoOptions{
		adminView:             true,
		includeUpstreamTaskID: true,
	})
	require.Len(t, items, 1)
	assert.Equal(t, "upstream-private-task-id", items[0].UpstreamTaskID)

	exportItems := tasksToDto([]*model.Task{task}, taskDtoOptions{
		adminView: true,
	})
	require.Len(t, exportItems, 1)
	assert.Empty(t, exportItems[0].UpstreamTaskID)
}

func TestTasksToDtoDoesNotPromoteLegacyUpstreamModelToPublicName(t *testing.T) {
	task := &model.Task{
		TaskID: "task_legacy",
		Properties: model.Properties{
			UpstreamModelName: "provider-secret-model",
		},
		Data: []byte(`{"model":"provider-secret-model"}`),
	}

	items := tasksToDto([]*model.Task{task}, taskDtoOptions{})
	require.Len(t, items, 1)
	assert.Empty(t, items[0].ModelName)
	assert.NotContains(t, string(items[0].Data), "provider-secret-model")

	properties, ok := items[0].Properties.(model.Properties)
	require.True(t, ok)
	assert.Empty(t, properties.UpstreamModelName)
}
