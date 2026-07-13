package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

type TaskSubmitResult struct {
	UpstreamTaskID string
	TaskData       []byte
	Platform       constant.TaskPlatform
	Quota          int
	//PerCallPrice   types.PriceData
}

// ResolveOriginTask 处理基于已有任务的提交（remix / continuation）：
// 查找原始任务、从中提取模型名称、将渠道锁定到原始任务的渠道
// （通过 info.LockedChannel，重试时复用同一渠道并轮换 key），
// 以及提取 OtherRatios（时长、分辨率）。
// 该函数在控制器的重试循环之前调用一次，其结果通过 info 字段和上下文持久化。
func ResolveOriginTask(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	// 检测 remix action
	path := c.Request.URL.Path
	if strings.Contains(path, "/v1/videos/") && strings.HasSuffix(path, "/remix") {
		info.Action = constant.TaskActionRemix
	}

	// 提取 remix 任务的 video_id
	if info.Action == constant.TaskActionRemix {
		videoID := c.Param("video_id")
		if strings.TrimSpace(videoID) == "" {
			return service.TaskErrorWrapperLocal(fmt.Errorf("video_id is required"), "invalid_request", http.StatusBadRequest)
		}
		info.OriginTaskID = videoID
	}

	if info.OriginTaskID == "" {
		return nil
	}

	// 查找原始任务
	originTask, exist, err := model.GetByTaskId(info.UserId, info.OriginTaskID)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_origin_task_failed", http.StatusInternalServerError)
	}
	if !exist {
		return service.TaskErrorWrapperLocal(errors.New("task_origin_not_exist"), "task_not_exist", http.StatusBadRequest)
	}

	// 从原始任务推导模型名称
	if info.OriginModelName == "" {
		if originTask.Properties.OriginModelName != "" {
			info.OriginModelName = originTask.Properties.OriginModelName
		} else if originTask.Properties.UpstreamModelName != "" {
			info.OriginModelName = originTask.Properties.UpstreamModelName
		} else {
			var taskData map[string]interface{}
			_ = common.Unmarshal(originTask.Data, &taskData)
			if m, ok := taskData["model"].(string); ok && m != "" {
				info.OriginModelName = m
			}
		}
	}

	// 锁定到原始任务的渠道（重试时复用同一渠道，轮换 key）
	ch, err := model.GetChannelById(originTask.ChannelId, true)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "channel_not_found", http.StatusBadRequest)
	}
	if ch.Status != common.ChannelStatusEnabled {
		return service.TaskErrorWrapperLocal(errors.New("the channel of the origin task is disabled"), "task_channel_disable", http.StatusBadRequest)
	}
	info.LockedChannel = ch

	if originTask.ChannelId != info.ChannelId {
		key, _, newAPIError := ch.GetNextEnabledKey()
		if newAPIError != nil {
			return service.TaskErrorWrapper(newAPIError, "channel_no_available_key", newAPIError.StatusCode)
		}
		common.SetContextKey(c, constant.ContextKeyChannelKey, key)
		common.SetContextKey(c, constant.ContextKeyChannelType, ch.Type)
		common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, ch.GetBaseURL())
		common.SetContextKey(c, constant.ContextKeyChannelId, originTask.ChannelId)

		info.ChannelBaseUrl = ch.GetBaseURL()
		info.ChannelId = originTask.ChannelId
		info.ChannelType = ch.Type
		info.ApiKey = key
	}

	// 提取 remix 参数（时长、分辨率 → OtherRatios）
	if info.Action == constant.TaskActionRemix {
		if originTask.PrivateData.BillingContext != nil {
			// 新的 remix 逻辑：直接从原始任务的 BillingContext 中提取 OtherRatios（如果存在）
			for s, f := range originTask.PrivateData.BillingContext.OtherRatios {
				info.PriceData.AddOtherRatio(s, f)
			}
		} else {
			// 旧的 remix 逻辑：直接从 task data 解析 seconds 和 size（如果存在）
			var taskData map[string]interface{}
			_ = common.Unmarshal(originTask.Data, &taskData)
			secondsStr, _ := taskData["seconds"].(string)
			seconds, _ := strconv.Atoi(secondsStr)
			if seconds <= 0 {
				seconds = 4
			}
			// 历史任务数据可能包含未经校验的时长，作为计费乘数前必须钳制
			if seconds > relaycommon.MaxTaskDurationSeconds {
				seconds = relaycommon.MaxTaskDurationSeconds
			}
			sizeStr, _ := taskData["size"].(string)
			info.PriceData.AddOtherRatio("seconds", float64(seconds))
			info.PriceData.AddOtherRatio("size", 1)
			if sizeStr == "1792x1024" || sizeStr == "1024x1792" {
				info.PriceData.AddOtherRatio("size", 1.666667)
			}
		}
	}

	return nil
}

// RelayTaskSubmit 完成 task 提交的全部流程（每次尝试调用一次）：
// 刷新渠道元数据 → 确定 platform/adaptor → 验证请求 →
// 估算计费(EstimateBilling) → 计算价格 → 预扣费（仅首次）→
// 构建/发送/解析上游请求 → 提交后计费调整(AdjustBillingOnSubmit)。
// 控制器负责 defer Refund 和成功后 Settle。
func RelayTaskSubmit(c *gin.Context, info *relaycommon.RelayInfo) (*TaskSubmitResult, *dto.TaskError) {
	info.InitChannelMeta(c)

	// 1. 确定 platform → 创建适配器 → 验证请求
	platform := constant.TaskPlatform(c.GetString("platform"))
	if platform == "" {
		platform = GetTaskPlatform(c)
	}
	adaptor := GetTaskAdaptor(platform)
	if adaptor == nil {
		return nil, service.TaskErrorWrapperLocal(fmt.Errorf("invalid api platform: %s", platform), "invalid_api_platform", http.StatusBadRequest)
	}
	adaptor.Init(info)
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		return nil, taskErr
	}

	// 2. 确定模型名称
	modelName := info.OriginModelName
	if modelName == "" {
		modelName = service.CoverTaskActionToModelName(platform, info.Action)
	}

	// 2.5 应用渠道的模型映射（与同步任务对齐）
	info.OriginModelName = modelName
	info.UpstreamModelName = modelName
	if err := helper.ModelMappedHelper(c, info, nil); err != nil {
		return nil, service.TaskErrorWrapperLocal(err, "model_mapping_failed", http.StatusBadRequest)
	}

	// 3. 预生成公开 task ID（仅首次）
	if info.PublicTaskID == "" {
		info.PublicTaskID = model.GenerateTaskID()
	}

	// 4. 价格计算：基础模型价格
	info.OriginModelName = modelName
	priceData, err := helper.ModelPriceHelperPerCall(c, info)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "model_price_error", http.StatusBadRequest)
	}
	info.PriceData = priceData

	// 5. 计费估算：让适配器根据用户请求提供 OtherRatios（时长、分辨率等）
	//    必须在 ModelPriceHelperPerCall 之后调用（它会重建 PriceData）。
	//    ResolveOriginTask 可能已在 remix 路径中预设了 OtherRatios，此处合并。
	if estimatedRatios := adaptor.EstimateBilling(c, info); len(estimatedRatios) > 0 {
		for k, v := range estimatedRatios {
			info.PriceData.AddOtherRatio(k, v)
		}
	}

	perCallBilling := ratio_setting.IsVideoBillingPerCall(modelName) ||
		(info.PriceData.UsePrice && !ratio_setting.HasVideoBillingMode(modelName))

	// 6. 将 OtherRatios 应用到基础额度；按次计费的视频任务不受 seconds 等倍率影响。
	if !perCallBilling {
		genSec := c.GetInt("generated_video_seconds")
		refSec := c.GetInt("reference_video_seconds")
		if genSec > 0 || refSec > 0 {
			// 视频按秒计费：把「生成秒」与「参考秒」拆开分别计价。
			// basePerSec 是每秒基础额度（已含 group ratio）；参考秒按用户级规则计价。
			basePerSec := float64(info.PriceData.Quota)
			relativeRatio := 1.0
			for key, ratio := range info.PriceData.OtherRatios() {
				if key != "seconds" && ratio != 1.0 {
					relativeRatio *= ratio
				}
			}
			genCost := basePerSec * float64(genSec) * relativeRatio
			refCost := referenceVideoCost(info.PriceData.VideoRefMode, info.PriceData.VideoRefValue, float64(refSec), basePerSec, relativeRatio, info.PriceData.GroupRatioInfo.GroupRatio, info.PriceData.VideoRefApplyGroupRatio)
			quota, clamp := common.QuotaFromFloatChecked(genCost + refCost)
			info.PriceData.Quota = quota
			noteTaskQuotaClamp(info, clamp)
			// 参考固定单价/总价可能在「免费基础模型」上产生正额度，
			// 必须清掉 FreeModel，否则预扣会被跳过导致漏扣。
			if info.PriceData.Quota > 0 {
				info.PriceData.FreeModel = false
			}
			// 用「等效秒数」回写 seconds 倍率，便于日志透明与潜在的 token 重算保持一致。
			if basePerSec > 0 && relativeRatio > 0 {
				effSec := (genCost + refCost) / basePerSec / relativeRatio
				info.PriceData.AddOtherRatio("seconds", effSec)
				c.Set("billable_video_seconds", int(effSec))
			}
		} else {
			// 回退：没有 generated/reference 秒数上下文（如 remix），按 OtherRatios 连乘。
			quotaWithRatios := info.PriceData.ApplyOtherRatiosToFloat(float64(info.PriceData.Quota))
			quota, clamp := common.QuotaFromFloatChecked(quotaWithRatios)
			info.PriceData.Quota = quota
			noteTaskQuotaClamp(info, clamp)
		}
	}

	// 7. 限量池检查 + 预扣费（仅首次 — 重试时 info.Billing 已存在，跳过）
	if info.Billing == nil {
		if apiErr := service.CheckAndConsumeModelQuotaPool(c, info); apiErr != nil {
			return nil, service.TaskErrorFromAPIError(apiErr)
		}
	}
	if info.Billing == nil && !info.PriceData.FreeModel {
		info.ForcePreConsume = true
		if apiErr := service.PreConsumeBilling(c, info.PriceData.Quota, info); apiErr != nil {
			return nil, service.TaskErrorFromAPIError(apiErr)
		}
	}
	if _, err := service.EnsureTaskSubmissionRecord(c, info, platform); err != nil {
		return nil, service.TaskErrorWrapper(err, "insert_task_failed", http.StatusInternalServerError)
	}

	// 8. 构建请求体
	requestBody, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "build_request_failed", http.StatusInternalServerError)
	}

	// 9. 发送请求
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}
	if resp != nil && resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return nil, service.TaskErrorWrapper(fmt.Errorf("%s", string(responseBody)), "fail_to_fetch_task", resp.StatusCode)
	}

	// 10. 返回 OtherRatios 给下游（header 必须在 DoResponse 写 body 之前设置）
	otherRatios := info.PriceData.OtherRatios()
	if otherRatios == nil {
		otherRatios = map[string]float64{}
	}
	ratiosJSON, _ := common.Marshal(otherRatios)
	c.Header("X-New-Api-Other-Ratios", string(ratiosJSON))

	// 11. 解析响应
	upstreamTaskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		return nil, taskErr
	}

	// 11. 提交后计费调整：让适配器根据上游实际返回调整 OtherRatios
	finalQuota := info.PriceData.Quota
	if adjustedRatios := adaptor.AdjustBillingOnSubmit(info, taskData); len(adjustedRatios) > 0 {
		if !perCallBilling {
			if adjustedQuota, ok := recalcQuotaFromRatios(info, adjustedRatios); ok {
				// 基于调整后的 ratios 重新计算 quota
				finalQuota = adjustedQuota
				info.PriceData.ReplaceOtherRatios(adjustedRatios)
				info.PriceData.Quota = finalQuota
			}
		} else {
			info.PriceData.ReplaceOtherRatios(adjustedRatios)
		}
	}

	return &TaskSubmitResult{
		UpstreamTaskID: upstreamTaskID,
		TaskData:       taskData,
		Platform:       platform,
		Quota:          finalQuota,
	}, nil
}

// referenceVideoCost 计算「参考视频秒数」那部分的额度（不含生成秒）。
//   - basePerSec：每秒基础额度（已含 group ratio）
//   - sizeRatio：分辨率倍率（仅作用于按秒的相对模式 factor/cap）
//   - groupRatio / applyGroupRatio：仅对 price/flat 生效——是否让固定参考价也乘分组倍率。
//
// 模式：
//
//	factor → 参考秒 × value（0=免费,0.5=半价）；天然跟随基础价/分组折扣
//	price  → 参考每秒固定单价（绝对值；applyGroupRatio=true 时再乘分组倍率）
//	flat   → 参考整段固定总价（绝对值；applyGroupRatio=true 时再乘分组倍率）
//	cap    → 参考秒数封顶为 value 秒；天然跟随基础价/分组折扣
//	""     → 参考秒按原价全额计
//
// 无参考秒（refSec<=0）时一律不收参考费。
func referenceVideoCost(mode string, value, refSec, basePerSec, sizeRatio, groupRatio float64, applyGroupRatio bool) float64 {
	if refSec <= 0 {
		return 0
	}
	groupMul := 1.0
	if applyGroupRatio {
		groupMul = groupRatio
	}
	switch mode {
	case ratio_setting.VideoRefModeFactor:
		return basePerSec * refSec * value * sizeRatio
	case ratio_setting.VideoRefModePrice:
		return value * common.QuotaPerUnit * refSec * groupMul
	case ratio_setting.VideoRefModeFlat:
		return value * common.QuotaPerUnit * groupMul
	case ratio_setting.VideoRefModeCap:
		if refSec > value {
			refSec = value
		}
		return basePerSec * refSec * sizeRatio
	default:
		return basePerSec * refSec * sizeRatio
	}
}

// recalcQuotaFromRatios 根据 adjustedRatios 重新计算 quota。
// 公式: baseQuota × ∏(ratio) — 其中 baseQuota 是不含 OtherRatios 的基础额度。
func recalcQuotaFromRatios(info *relaycommon.RelayInfo, ratios map[string]float64) (int, bool) {
	// 从 PriceData 获取不含 OtherRatios 的基础价格
	baseQuota := info.PriceData.RemoveOtherRatiosFromFloat(float64(info.PriceData.Quota))
	priceData := info.PriceData
	if !priceData.ReplaceOtherRatios(ratios) {
		return 0, false
	}
	// 应用新的 ratios
	result := priceData.ApplyOtherRatiosToFloat(baseQuota)
	quota, clamp := common.QuotaFromFloatChecked(result)
	noteTaskQuotaClamp(info, clamp)
	return quota, true
}

// noteTaskQuotaClamp records the first quota saturation event onto the task's
// RelayInfo so LogTaskConsumption can surface it on the submit log's
// admin_info. First non-nil clamp wins.
func noteTaskQuotaClamp(info *relaycommon.RelayInfo, clamp *common.QuotaClamp) {
	if clamp == nil || info == nil {
		return
	}
	if info.QuotaClamp == nil {
		info.QuotaClamp = clamp
	}
}

var fetchRespBuilders = map[int]func(c *gin.Context) (respBody []byte, taskResp *dto.TaskError){
	relayconstant.RelayModeSunoFetchByID:  sunoFetchByIDRespBodyBuilder,
	relayconstant.RelayModeSunoFetch:      sunoFetchRespBodyBuilder,
	relayconstant.RelayModeVideoFetchByID: videoFetchByIDRespBodyBuilder,
}

func RelayTaskFetch(c *gin.Context, relayMode int) (taskResp *dto.TaskError) {
	respBuilder, ok := fetchRespBuilders[relayMode]
	if !ok {
		taskResp = service.TaskErrorWrapperLocal(errors.New("invalid_relay_mode"), "invalid_relay_mode", http.StatusBadRequest)
	}

	respBody, taskErr := respBuilder(c)
	if taskErr != nil {
		return taskErr
	}
	if len(respBody) == 0 {
		respBody = []byte("{\"code\":\"success\",\"data\":null}")
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	_, err := io.Copy(c.Writer, bytes.NewBuffer(respBody))
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError)
		return
	}
	return
}

func sunoFetchRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	userId := c.GetInt("id")
	var condition = struct {
		IDs    []any  `json:"ids"`
		Action string `json:"action"`
	}{}
	err := c.BindJSON(&condition)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "invalid_request", http.StatusBadRequest)
		return
	}
	var tasks []any
	if len(condition.IDs) > 0 {
		taskModels, err := model.GetByTaskIds(userId, condition.IDs)
		if err != nil {
			taskResp = service.TaskErrorWrapper(err, "get_tasks_failed", http.StatusInternalServerError)
			return
		}
		for _, task := range taskModels {
			tasks = append(tasks, TaskModel2Dto(task))
		}
	} else {
		tasks = make([]any, 0)
	}
	respBody, err = common.Marshal(dto.TaskResponse[[]any]{
		Code: "success",
		Data: tasks,
	})
	return
}

func sunoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("id")
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	respBody, err = common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: TaskModel2Dto(originTask),
	})
	return
}

func videoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("task_id")
	if taskId == "" {
		taskId = c.GetString("task_id")
	}
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	isOpenAIVideoAPI := strings.HasPrefix(c.Request.RequestURI, "/v1/videos/")

	// Gemini/Vertex 支持实时查询：用户 fetch 时直接从上游拉取最新状态
	if realtimeResp := tryRealtimeFetch(c.Request.Context(), originTask, isOpenAIVideoAPI); len(realtimeResp) > 0 {
		respBody = realtimeResp
		return
	}

	// /v1/videos/:task_id 统一返回精简的 VideoTaskPublicDto，不再按渠道透传上游
	// 原生结构，避免 xAI 等上游把 usage/object/video 等内部字段直接暴露给调用方。
	// 视频地址由 TaskModel2Dto 从 task.Data 递归归一化进 result_url/url/video_url。
	if isOpenAIVideoAPI {
		respBody, err = common.Marshal(TaskModel2PublicVideoDto(originTask))
		if err != nil {
			taskResp = service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
		}
		return
	}

	// 通用 TaskDto 格式
	respBody, err = common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: TaskModel2Dto(originTask),
	})
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	return
}

// tryRealtimeFetch 尝试从上游实时拉取 Gemini/Vertex 任务状态。
// 仅当渠道类型为 Gemini 或 Vertex 时触发；其他渠道或出错时返回 nil。
// 当非 OpenAI Video API 时，还会构建自定义格式的响应体。
func tryRealtimeFetch(ctx context.Context, task *model.Task, isOpenAIVideoAPI bool) []byte {
	channelModel, err := model.GetChannelById(task.ChannelId, true)
	if err != nil {
		return nil
	}
	if channelModel.Type != constant.ChannelTypeVertexAi && channelModel.Type != constant.ChannelTypeGemini {
		return nil
	}

	baseURL := constant.ChannelBaseURLs[channelModel.Type]
	if channelModel.GetBaseURL() != "" {
		baseURL = channelModel.GetBaseURL()
	}
	proxy := channelModel.GetSetting().Proxy
	adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(channelModel.Type)))
	if adaptor == nil {
		return nil
	}

	resp, err := adaptor.FetchTask(baseURL, channelModel.Key, map[string]any{
		"task_id": task.GetUpstreamTaskID(),
		"action":  task.Action,
	}, proxy)
	if err != nil || resp == nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	ti, err := adaptor.ParseTaskResult(body)
	if err != nil || ti == nil {
		return nil
	}

	snap := task.Snapshot()

	// 将上游最新状态更新到 task
	if ti.Status != "" {
		task.Status = model.TaskStatus(ti.Status)
	}
	if ti.Progress != "" {
		task.Progress = ti.Progress
	}
	if strings.HasPrefix(ti.Url, "data:") {
		// data: URI — kept in Data, not ResultURL
	} else if ti.Url != "" {
		task.PrivateData.ResultURL = ti.Url
	}

	if task.Status == model.TaskStatusFailure && snap.Status != model.TaskStatusFailure {
		reason := ti.Reason
		if reason == "" {
			reason = "upstream task failed"
		}
		_, _ = service.FinalizeTaskFailure(ctx, task, snap.Status, reason)
	} else if !snap.Equal(task.Snapshot()) {
		_, _ = task.UpdateWithStatus(snap.Status)
	}

	// OpenAI Video API 由调用者的 ConvertToOpenAIVideo 分支处理
	if isOpenAIVideoAPI {
		return nil
	}

	// 非 OpenAI Video API: 构建自定义格式响应
	format := detectVideoFormat(body)
	out := map[string]any{
		"error":    nil,
		"format":   format,
		"metadata": nil,
		"status":   mapTaskStatusToSimple(task.Status),
		"task_id":  task.TaskID,
		"url":      task.GetResultURL(),
	}
	respBody, _ := common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: out,
	})
	return respBody
}

// detectVideoFormat 从 Gemini/Vertex 原始响应中探测视频格式
func detectVideoFormat(rawBody []byte) string {
	var raw map[string]any
	if err := common.Unmarshal(rawBody, &raw); err != nil {
		return "mp4"
	}
	respObj, ok := raw["response"].(map[string]any)
	if !ok {
		return "mp4"
	}
	vids, ok := respObj["videos"].([]any)
	if !ok || len(vids) == 0 {
		return "mp4"
	}
	v0, ok := vids[0].(map[string]any)
	if !ok {
		return "mp4"
	}
	mt, ok := v0["mimeType"].(string)
	if !ok || mt == "" || strings.Contains(mt, "mp4") {
		return "mp4"
	}
	return mt
}

// mapTaskStatusToSimple 将内部 TaskStatus 映射为简化状态字符串
func mapTaskStatusToSimple(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	case model.TaskStatusQueued, model.TaskStatusSubmitted:
		return "queued"
	default:
		return "processing"
	}
}

func TaskModel2Dto(task *model.Task) *dto.TaskDto {
	resultURL := taskPublicResultURL(task)
	return &dto.TaskDto{
		ID:               task.ID,
		CreatedAt:        task.CreatedAt,
		UpdatedAt:        task.UpdatedAt,
		TaskID:           task.TaskID,
		Platform:         string(task.Platform),
		UserId:           task.UserId,
		Group:            task.Group,
		ChannelId:        task.ChannelId,
		ChannelName:      task.ChannelName,
		Quota:            task.Quota,
		RefundQuota:      task.PrivateData.RefundQuota,
		Action:           task.Action,
		Status:           string(task.Status),
		FailReason:       task.FailReason,
		ResultURL:        resultURL,
		URL:              resultURL,
		VideoURL:         resultURL,
		SubmitTime:       task.SubmitTime,
		StartTime:        task.StartTime,
		FinishTime:       task.FinishTime,
		Progress:         task.Progress,
		Properties:       task.Properties,
		Username:         task.Username,
		ModelName:        taskModelName(task),
		VideoDuration:    taskVideoDuration(task),
		Data:             taskDataWithResultURL(task.Data, resultURL, task.TaskID),
		Timestamp2String: taskTimestampString(task.CreatedAt),
		Key:              strconv.FormatInt(task.ID, 10),
	}
}

// TaskModel2PublicVideoDto 构建 /v1/videos/{task_id} 的对外精简响应，
// 复用 TaskModel2Dto 的字段计算逻辑，但只保留调用方需要的字段，
// 不暴露 platform / user_id / group / channel_id / quota 等内部信息。
func TaskModel2PublicVideoDto(task *model.Task) *dto.VideoTaskPublicDto {
	full := TaskModel2Dto(task)
	seconds, size := extractVideoSecondsSize(task.Data)
	if seconds == "" && full.VideoDuration > 0 {
		seconds = strconv.Itoa(full.VideoDuration)
	}
	out := &dto.VideoTaskPublicDto{
		ID:     full.TaskID,
		Object: "video",
		Model:  full.ModelName,
		// status 映射为 OpenAI 小写（queued/in_progress/completed/failed），
		// 让 OpenAI SDK 与下游 new-api 的 sora 解析器都能正确识别任务完成/失败。
		Status:           task.Status.ToVideoStatus(),
		Seconds:          seconds,
		Size:             size,
		CreatedAt:        full.CreatedAt,
		UpdatedAt:        full.UpdatedAt,
		TaskID:           full.TaskID,
		Action:           full.Action,
		FailReason:       full.FailReason,
		ResultURL:        full.ResultURL,
		URL:              full.URL,
		VideoURL:         full.VideoURL,
		SubmitTime:       full.SubmitTime,
		StartTime:        full.StartTime,
		FinishTime:       full.FinishTime,
		Progress:         publicVideoProgress(task, full.Progress),
		Properties:       full.Properties,
		ModelName:        full.ModelName,
		VideoDuration:    full.VideoDuration,
		Data:             stripTaskDataSensitiveFields(full.Data),
		Timestamp2String: full.Timestamp2String,
	}
	if task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
		out.CompletedAt = full.FinishTime
	}
	if task.Status == model.TaskStatusFailure {
		message := full.FailReason
		if message == "" {
			message = "task failed"
		}
		out.Error = &dto.OpenAIVideoError{Message: message}
	}
	return out
}

// extractVideoSecondsSize 从上游 data 里取 OpenAI 视频对象的 seconds/size 字段（若有），
// 用于补齐 OpenAI SDK 兼容字段。
func extractVideoSecondsSize(data json.RawMessage) (string, string) {
	if len(data) == 0 {
		return "", ""
	}
	var m map[string]any
	if err := common.Unmarshal(data, &m); err != nil {
		return "", ""
	}
	seconds := ""
	if v, ok := m["seconds"]; ok && v != nil {
		seconds = strings.TrimSpace(fmt.Sprintf("%v", v))
	}
	size := ""
	if v, ok := m["size"].(string); ok {
		size = strings.TrimSpace(v)
	}
	return seconds, size
}

// stripTaskDataSensitiveFields 从对外返回的 data 中移除上游内部字段（如 usage 计费信息），
// 避免把上游成本（xAI 的 usage.cost_in_usd_ticks 等）透传给调用方。
func stripTaskDataSensitiveFields(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	var m map[string]any
	if err := common.Unmarshal(data, &m); err != nil {
		return data
	}
	if _, ok := m["usage"]; !ok {
		return data
	}
	delete(m, "usage")
	b, err := common.Marshal(m)
	if err != nil {
		return data
	}
	return json.RawMessage(b)
}

// publicVideoProgress 计算对外进度：终态（成功/失败）固定 100，
// 否则解析存储的进度字符串。规避部分适配器在完成时不回写 progress 导致显示 0 的问题。
func publicVideoProgress(task *model.Task, progress string) int {
	if task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
		return 100
	}
	return parseProgressPercent(progress)
}

// parseProgressPercent 将 "100%" / "50%" 这类进度字符串转为整数 0-100，
// 无法解析时返回 0。
func parseProgressPercent(progress string) int {
	s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(progress), "%"))
	if s == "" {
		return 0
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return v
}

func taskPublicResultURL(task *model.Task) string {
	if directURL := extractTaskDirectVideoURL(task.Data, task.TaskID); directURL != "" {
		return directURL
	}
	resultURL := strings.TrimSpace(task.GetResultURL())
	if resultURL == "" || !isTaskProxyContentURL(resultURL, task.TaskID) {
		return resultURL
	}
	return ""
}

func taskDataWithResultURL(data json.RawMessage, resultURL string, taskID string) json.RawMessage {
	out := map[string]any{}
	if len(data) > 0 {
		if err := common.Unmarshal(data, &out); err != nil {
			if resultURL == "" {
				return data
			}
			out = map[string]any{}
		}
	}
	if out == nil {
		out = map[string]any{}
	}
	if resultURL == "" {
		for _, key := range []string{"video_url", "result_url", "url"} {
			if value, ok := out[key].(string); ok && !isUsableVideoURL(value, taskID) {
				delete(out, key)
			}
		}
		if len(out) == 0 {
			return data
		}
		b, err := common.Marshal(out)
		if err != nil {
			return data
		}
		return json.RawMessage(b)
	}
	out["result_url"] = resultURL
	out["url"] = resultURL
	out["video_url"] = resultURL
	b, err := common.Marshal(out)
	if err != nil {
		return data
	}
	return json.RawMessage(b)
}

func taskModelName(task *model.Task) string {
	if task.Properties.OriginModelName != "" {
		return task.Properties.OriginModelName
	}
	if task.Properties.UpstreamModelName != "" {
		return task.Properties.UpstreamModelName
	}
	var data map[string]any
	if err := common.Unmarshal(task.Data, &data); err != nil {
		return ""
	}
	if modelName, ok := data["model"].(string); ok {
		return modelName
	}
	return ""
}

func taskVideoDuration(task *model.Task) int {
	var maxDuration int
	var data map[string]any
	if err := common.Unmarshal(task.Data, &data); err == nil {
		for _, key := range []string{"video_duration", "duration", "seconds"} {
			if duration := intFromAny(data[key]); duration > maxDuration {
				maxDuration = duration
			}
		}
	}
	// 上游未回写时长时（如刚提交、data 为空、或上游把时长放在嵌套结构里），
	// 回退到提交时记录的请求时长，保证任务创建后即可展示时长。
	if maxDuration == 0 && task.Properties.VideoSeconds > 0 {
		maxDuration = task.Properties.VideoSeconds
	}
	return maxDuration
}

func intFromAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	default:
		return 0
	}
}

func taskTimestampString(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04:05")
}

func extractTaskDirectVideoURL(data []byte, taskID string) string {
	if len(data) == 0 {
		return ""
	}
	var payload any
	if err := common.Unmarshal(data, &payload); err != nil {
		return ""
	}
	return findVideoURL(payload, taskID)
}

func findVideoURL(v any, taskID string) string {
	switch x := v.(type) {
	case map[string]any:
		for _, key := range []string{"video_url", "result_url", "url", "download_url", "file_url"} {
			if value, ok := x[key].(string); ok && isUsableVideoURL(value, taskID) {
				return strings.TrimSpace(value)
			}
		}
		for _, value := range x {
			if url := findVideoURL(value, taskID); url != "" {
				return url
			}
		}
	case []any:
		for _, value := range x {
			if url := findVideoURL(value, taskID); url != "" {
				return url
			}
		}
	}
	return ""
}

func isUsableVideoURL(value string, taskID string) bool {
	value = strings.TrimSpace(value)
	if value == "" || isTaskProxyContentURL(value, taskID) || strings.HasSuffix(strings.TrimRight(value, "/"), "/content") {
		return false
	}
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "data:")
}

func isTaskProxyContentURL(value string, taskID string) bool {
	if strings.TrimSpace(value) == "" || strings.TrimSpace(taskID) == "" {
		return false
	}
	return strings.Contains(value, "/v1/videos/"+taskID+"/content")
}
