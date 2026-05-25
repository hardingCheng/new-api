package xai

import (
	"github.com/QuantumNous/new-api/relay/channel/task/sora"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

type TaskAdaptor struct {
	sora.TaskAdaptor
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	if !taskcommon.IsGrokImagineVideoModel(req.Model) &&
		!taskcommon.IsGrokImagineVideoModel(info.OriginModelName) &&
		!taskcommon.IsGrokImagineVideoModel(info.UpstreamModelName) {
		return nil
	}
	seconds := relaycommon.TaskDurationSeconds(req)
	if seconds <= 0 {
		return nil
	}
	return map[string]float64{"seconds": float64(seconds)}
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}
