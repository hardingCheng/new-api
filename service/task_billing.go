package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// LogTaskConsumption 记录任务消费日志和统计信息（仅记录，不涉及实际扣费）。
// 实际扣费已由 BillingSession（PreConsumeBilling + SettleBilling）完成。
func LogTaskConsumption(c *gin.Context, info *relaycommon.RelayInfo) {
	tokenName := c.GetString("token_name")
	logContent := fmt.Sprintf("操作 %s", info.Action)
	videoBillingMode := ratio_setting.GetVideoBillingMode(info.EffectiveBillingModelName())
	// 支持任务仅按次计费
	if videoBillingMode == ratio_setting.VideoBillingModePerCall {
		logContent = fmt.Sprintf("%s，按次计费", logContent)
	} else {
		if otherRatios := info.PriceData.OtherRatios(); len(otherRatios) > 0 {
			var contents []string
			for key, ra := range otherRatios {
				if 1.0 != ra {
					contents = append(contents, fmt.Sprintf("%s: %.2f", key, ra))
				}
			}
			if len(contents) > 0 {
				logContent = fmt.Sprintf("%s, 计算参数：%s", logContent, strings.Join(contents, ", "))
			}
		}
	}
	other := make(map[string]interface{})
	other["is_task"] = true
	other["request_path"] = c.Request.URL.Path
	other["video_billing_mode"] = videoBillingMode
	other["model_price"] = info.PriceData.ModelPrice
	if info.PriceData.ModelRatio > 0 {
		other["model_ratio"] = info.PriceData.ModelRatio
	}
	other["group_ratio"] = info.PriceData.GroupRatioInfo.GroupRatio
	if info.PriceData.GroupRatioInfo.HasSpecialRatio {
		other["user_group_ratio"] = info.PriceData.GroupRatioInfo.GroupSpecialRatio
	}
	if info.IsModelMapped {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = info.UpstreamModelName
	}
	appendAdminBillingRules(other, info.UserPricingOverrides, info.ModelQuotaPools)
	if otherRatios := info.PriceData.OtherRatios(); len(otherRatios) > 0 {
		for k, v := range otherRatios {
			other[k] = v
		}
	}
	if generatedSeconds := c.GetInt("generated_video_seconds"); generatedSeconds > 0 {
		other["generated_video_seconds"] = generatedSeconds
	}
	if referenceSeconds := c.GetInt("reference_video_seconds"); referenceSeconds > 0 {
		other["reference_video_seconds"] = referenceSeconds
	}
	if billableSeconds := c.GetInt("billable_video_seconds"); billableSeconds > 0 {
		other["billable_video_seconds"] = billableSeconds
	}
	attachQuotaSaturation(c, info, other)
	model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
		ChannelId: info.ChannelId,
		ModelName: info.OriginModelName,
		TokenName: tokenName,
		Quota:     info.PriceData.Quota,
		Content:   logContent,
		TokenId:   info.TokenId,
		Group:     info.UsingGroup,
		Other:     other,
	})
	model.UpdateUserUsedQuotaAndRequestCount(info.UserId, info.PriceData.Quota)
	model.UpdateChannelUsedQuota(info.ChannelId, info.PriceData.Quota)
}

func EnsureTaskSubmissionRecord(c *gin.Context, info *relaycommon.RelayInfo, platform constant.TaskPlatform) (*model.Task, error) {
	if info == nil || strings.TrimSpace(info.PublicTaskID) == "" {
		return nil, fmt.Errorf("public task id is required")
	}
	if existing, found, err := model.GetByTaskId(info.UserId, info.PublicTaskID); err != nil {
		return nil, err
	} else if found {
		if existing.Status != model.TaskStatusSubmitting {
			return nil, fmt.Errorf("task submission record %s has status %s", info.PublicTaskID, existing.Status)
		}
		return existing, nil
	}

	task := model.InitTask(platform, info)
	task.Status = model.TaskStatusSubmitting
	task.BillingStatus = model.TaskBillingStatusActive
	task.Quota = info.PriceData.Quota
	task.Action = info.Action
	task.PrivateData.BillingSource = info.BillingSource
	task.PrivateData.SubscriptionId = info.SubscriptionId
	task.PrivateData.TokenId = info.TokenId
	task.PrivateData.NodeName = common.NodeName
	task.PrivateData.BillingContext = taskBillingContextFromRelayInfo(info)
	if c != nil {
		if referenceSeconds := c.GetInt("reference_video_seconds"); referenceSeconds > 0 {
			task.Properties.HasReferenceVideo = true
			task.Properties.ReferenceVideoSeconds = referenceSeconds
		}
		if generatedSeconds := c.GetInt("generated_video_seconds"); generatedSeconds > 0 {
			task.Properties.VideoSeconds = generatedSeconds
		}
	}
	if err := task.Insert(); err != nil {
		return nil, err
	}
	return task, nil
}

func taskBillingContextFromRelayInfo(info *relaycommon.RelayInfo) *model.TaskBillingContext {
	return &model.TaskBillingContext{
		ModelPrice:                 info.PriceData.ModelPrice,
		GroupRatio:                 info.PriceData.GroupRatioInfo.GroupRatio,
		ModelRatio:                 info.PriceData.ModelRatio,
		OtherRatios:                info.PriceData.OtherRatios(),
		OriginModelName:            info.OriginModelName,
		VideoBillingMode:           ratio_setting.GetVideoBillingMode(info.EffectiveBillingModelName()),
		UserPricingOverrides:       info.UserPricingOverrides,
		ModelQuotaPools:            info.ModelQuotaPools,
		PerCallBilling:             ratio_setting.IsVideoBillingPerCall(info.EffectiveBillingModelName()) || (info.PriceData.UsePrice && !ratio_setting.HasVideoBillingMode(info.EffectiveBillingModelName())),
		SubmissionRequestID:        info.RequestId,
		SubmissionPreConsumedQuota: info.FinalPreConsumedQuota,
		SubmissionTokenBilling:     !info.IsPlayground,
	}
}

func MarkTaskSubmissionFailed(ctx context.Context, userID int, publicTaskID string, reason string) (bool, error) {
	task, found, err := model.GetByTaskId(userID, publicTaskID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	if task.Status != model.TaskStatusSubmitting {
		return true, nil
	}
	_, err = FinalizeTaskFailure(ctx, task, model.TaskStatusSubmitting, reason)
	return true, err
}

func CompleteTaskSubmissionRecord(c *gin.Context, info *relaycommon.RelayInfo, resultPlatform constant.TaskPlatform, upstreamTaskID string, taskData []byte, quota int) error {
	if info == nil || strings.TrimSpace(info.PublicTaskID) == "" {
		return fmt.Errorf("public task id is required")
	}
	task, found, err := model.GetByTaskId(info.UserId, info.PublicTaskID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("task submission record %s not found", info.PublicTaskID)
	}
	if task.Status != model.TaskStatusSubmitting {
		return fmt.Errorf("task submission record %s has status %s", info.PublicTaskID, task.Status)
	}

	task.Platform = resultPlatform
	task.Status = model.TaskStatusNotStart
	task.Progress = "0%"
	task.BillingStatus = model.TaskBillingStatusActive
	task.Quota = quota
	task.Action = info.Action
	task.Data = taskData
	task.PrivateData.UpstreamTaskID = upstreamTaskID
	task.PrivateData.UsesPublicTaskID = true
	task.PrivateData.BillingSource = info.BillingSource
	task.PrivateData.SubscriptionId = info.SubscriptionId
	task.PrivateData.TokenId = info.TokenId
	task.PrivateData.NodeName = common.NodeName
	task.PrivateData.BillingContext = taskBillingContextFromRelayInfo(info)
	if task.PrivateData.BillingContext != nil && (info.Billing != nil || quota != info.FinalPreConsumedQuota) {
		task.PrivateData.BillingContext.SubmissionActualQuota = quota
		task.PrivateData.BillingContext.SubmissionSettlementPending = true
		task.BillingStatus = model.TaskBillingStatusSettlementPending
	}
	if c != nil {
		if referenceSeconds := c.GetInt("reference_video_seconds"); referenceSeconds > 0 {
			task.Properties.HasReferenceVideo = true
			task.Properties.ReferenceVideoSeconds = referenceSeconds
		}
		if generatedSeconds := c.GetInt("generated_video_seconds"); generatedSeconds > 0 {
			task.Properties.VideoSeconds = generatedSeconds
		}
	}
	won, err := task.UpdateWithStatus(model.TaskStatusSubmitting)
	if err != nil {
		return err
	}
	if !won {
		return fmt.Errorf("task submission record %s was updated concurrently", info.PublicTaskID)
	}
	return nil
}

func SyncTaskSubmissionBillingContext(info *relaycommon.RelayInfo) error {
	if info == nil || strings.TrimSpace(info.PublicTaskID) == "" {
		return nil
	}
	task, found, err := model.GetByTaskId(info.UserId, info.PublicTaskID)
	if err != nil || !found {
		return err
	}
	next := taskBillingContextFromRelayInfo(info)
	if current := task.PrivateData.BillingContext; current != nil {
		next.SubmissionActualQuota = current.SubmissionActualQuota
		next.SubmissionSettlementPending = current.SubmissionSettlementPending
		next.SubmissionSettled = current.SubmissionSettled
		next.SettlementHasActualQuota = current.SettlementHasActualQuota
		next.SettlementActualQuota = current.SettlementActualQuota
		next.SettlementActualTokens = current.SettlementActualTokens
		next.SettlementReason = current.SettlementReason
		next.SettlementQuotaClamp = current.SettlementQuotaClamp
	}
	task.PrivateData.BillingContext = next
	return task.UpdatePrivateData()
}

func CompleteTaskSubmissionSettlement(userID int, publicTaskID string) error {
	task, found, err := model.GetByTaskId(userID, publicTaskID)
	if err != nil || !found {
		return err
	}
	if task.PrivateData.BillingContext == nil || !task.PrivateData.BillingContext.SubmissionSettlementPending {
		return nil
	}
	task.PrivateData.BillingContext.SubmissionSettlementPending = false
	task.PrivateData.BillingContext.SubmissionActualQuota = 0
	task.PrivateData.BillingContext.SubmissionSettled = true
	if task.BillingStatus == model.TaskBillingStatusSettlementPending {
		task.BillingStatus = model.TaskBillingStatusActive
	}
	return task.UpdateBillingState()
}

// taskBillingOther 从 task 的 BillingContext 构建日志 Other 字段。
func taskBillingOther(task *model.Task) map[string]interface{} {
	other := make(map[string]interface{})
	if bc := task.PrivateData.BillingContext; bc != nil {
		if bc.VideoBillingMode != "" {
			other["video_billing_mode"] = bc.VideoBillingMode
		}
		other["model_price"] = bc.ModelPrice
		if bc.ModelRatio > 0 {
			other["model_ratio"] = bc.ModelRatio
		}
		other["group_ratio"] = bc.GroupRatio
		appendAdminBillingRules(other, bc.UserPricingOverrides, bc.ModelQuotaPools)
		if priceData := taskBillingContextPriceData(bc); priceData != nil {
			for k, v := range priceData.OtherRatios() {
				other[k] = v
			}
		}
	}
	props := task.Properties
	if props.UpstreamModelName != "" && props.UpstreamModelName != props.OriginModelName {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = props.UpstreamModelName
	}
	return other
}

func taskBillingContextPriceData(bc *model.TaskBillingContext) *types.PriceData {
	if bc == nil || len(bc.OtherRatios) == 0 {
		return nil
	}
	priceData := &types.PriceData{}
	if !priceData.ReplaceOtherRatios(bc.OtherRatios) {
		return nil
	}
	return priceData
}

// taskModelName 从 BillingContext 或 Properties 中获取模型名称。
func taskModelName(task *model.Task) string {
	if bc := task.PrivateData.BillingContext; bc != nil && bc.OriginModelName != "" {
		return bc.OriginModelName
	}
	return task.Properties.OriginModelName
}

// RefundTaskQuota handles terminal task failure cleanup. Pool release and the
// durable financial adjustment are attempted independently; the task remains
// pending until both succeed, and retries cannot issue a double refund.
func RefundTaskQuota(ctx context.Context, task *model.Task, reason string) error {
	if task == nil || task.BillingStatus == model.TaskBillingStatusRefunded {
		return nil
	}
	poolErr := RollbackTaskModelQuotaPool(task)
	if err := applyPendingTaskSubmissionSettlement(ctx, task); err != nil {
		return errors.Join(poolErr, err)
	}
	submissionRefunded := false
	if bc := task.PrivateData.BillingContext; bc != nil && !bc.SubmissionSettled {
		var err error
		submissionRefunded, _, err = finalizeTaskSubmissionReservation(ctx, task, 0, true)
		if err != nil {
			return errors.Join(poolErr, err)
		}
		if submissionRefunded {
			bc.SubmissionActualQuota = 0
			bc.SubmissionSettlementPending = false
			bc.SubmissionSettled = true
		}
	}
	quota := task.Quota
	if quota == 0 {
		if poolErr != nil {
			return poolErr
		}
		if task.ID > 0 {
			task.BillingStatus = model.TaskBillingStatusRefunded
			return task.UpdateBillingState()
		}
		task.BillingStatus = model.TaskBillingStatusRefunded
		return nil
	}

	if !submissionRefunded {
		adjustment, err := ensureTaskBillingAdjustment(task, "task-refund:"+task.TaskID, -quota, -quota)
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("创建退款调整失败 task %s: %s", task.TaskID, err.Error()))
			return errors.Join(poolErr, err)
		}
		if err = model.ApplyBillingAdjustment(adjustment.ID); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("退款调整失败，等待后台重试 task %s: %s", task.TaskID, err.Error()))
			return errors.Join(poolErr, err)
		}
	}
	if poolErr != nil {
		return poolErr
	}
	if task.ID > 0 {
		task.BillingStatus = model.TaskBillingStatusRefunded
		if err := task.UpdateBillingState(); err != nil {
			return err
		}
	} else {
		task.BillingStatus = model.TaskBillingStatusRefunded
	}

	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["reason"] = reason
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   model.LogTypeRefund,
		Content:   "",
		ChannelId: task.ChannelId,
		ModelName: taskModelName(task),
		Quota:     quota,
		TokenId:   task.PrivateData.TokenId,
		Group:     task.Group,
		Other:     other,
	})
	return nil
}

func ensureTaskBillingAdjustment(task *model.Task, key string, fundingDelta int, tokenDelta int) (*model.BillingAdjustment, error) {
	fundingSource := task.PrivateData.BillingSource
	if fundingSource == "" {
		fundingSource = BillingSourceWallet
	}
	return model.EnsureBillingAdjustment(model.BillingAdjustmentParams{
		AdjustmentKey:  key,
		UserID:         task.UserId,
		TokenID:        task.PrivateData.TokenId,
		SubscriptionID: task.PrivateData.SubscriptionId,
		FundingSource:  fundingSource,
		FundingDelta:   fundingDelta,
		TokenDelta:     tokenDelta,
	})
}

func FinalizeTaskFailure(ctx context.Context, task *model.Task, fromStatus model.TaskStatus, reason string) (bool, error) {
	if task == nil {
		return false, nil
	}
	task.Status = model.TaskStatusFailure
	task.Progress = taskcommon.ProgressComplete
	if task.FinishTime == 0 {
		task.FinishTime = time.Now().Unix()
	}
	task.FailReason = reason
	task.BillingStatus = model.TaskBillingStatusRefundPending
	won, err := task.UpdateWithStatus(fromStatus)
	if err != nil || !won {
		return won, err
	}
	if err = RefundTaskQuota(ctx, task, reason); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("task failure compensation queued task %s: %s", task.TaskID, err.Error()))
	}
	return true, nil
}

// RecalculateTaskQuota 通用的异步差额结算。
// actualQuota 是任务完成后的实际应扣额度，与预扣额度 (task.Quota) 做差额结算。
// reason 用于日志记录（例如 "token重算" 或 "adaptor调整"）。
// clamps 可选：若计算 actualQuota 时发生额度饱和，将其记入日志 admin_info（仅管理员可见）。
func RecalculateTaskQuota(ctx context.Context, task *model.Task, actualQuota int, reason string, clamps ...*common.QuotaClamp) bool {
	if actualQuota <= 0 {
		return false
	}
	preConsumedQuota := task.Quota
	quotaDelta := actualQuota - preConsumedQuota

	if quotaDelta == 0 {
		if err := AdjustTaskModelQuotaPoolQuota(task, actualQuota); err != nil {
			logger.LogError(ctx, fmt.Sprintf("任务 %s 限量池结算入队失败: %s", task.TaskID, err.Error()))
			return false
		}
		logger.LogInfo(ctx, fmt.Sprintf("任务 %s 预扣费准确（%s，%s）",
			task.TaskID, logger.LogQuota(actualQuota), reason))
		return true
	}

	logger.LogInfo(ctx, fmt.Sprintf("任务 %s 差额结算：delta=%s（实际：%s，预扣：%s，%s）",
		task.TaskID,
		logger.LogQuota(quotaDelta),
		logger.LogQuota(actualQuota),
		logger.LogQuota(preConsumedQuota),
		reason,
	))

	adjustment, err := ensureTaskBillingAdjustment(task, "task-settle:"+task.TaskID, quotaDelta, quotaDelta)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("创建差额结算调整失败 task %s: %s", task.TaskID, err.Error()))
		return false
	}
	billingPending := false
	appliedNow, applyErr := model.ApplyBillingAdjustmentChecked(adjustment.ID)
	if applyErr != nil {
		billingPending = true
		logger.LogError(ctx, fmt.Sprintf("差额结算等待后台重试 task %s: %s", task.TaskID, applyErr.Error()))
	}

	task.Quota = actualQuota
	poolSettled := true
	if err := AdjustTaskModelQuotaPoolQuota(task, actualQuota); err != nil {
		logger.LogError(ctx, fmt.Sprintf("任务 %s 限量池结算入队失败: %s", task.TaskID, err.Error()))
		poolSettled = false
	}
	quotaPersisted := true
	if err := task.UpdateQuota(); err != nil {
		logger.LogError(ctx, fmt.Sprintf("差额结算回写 quota 失败 task %s: %s", task.TaskID, err.Error()))
		if task.ID > 0 {
			quotaPersisted = false
		}
	}
	if !appliedNow {
		return poolSettled && quotaPersisted
	}

	var logType int
	var logQuota int
	if quotaDelta > 0 {
		logType = model.LogTypeConsume
		logQuota = quotaDelta
		model.UpdateUserUsedQuotaAndRequestCount(task.UserId, quotaDelta)
		model.UpdateChannelUsedQuota(task.ChannelId, quotaDelta)
	} else {
		logType = model.LogTypeRefund
		logQuota = -quotaDelta
	}
	other := taskBillingOther(task)
	other["task_id"] = task.TaskID
	other["pre_consumed_quota"] = preConsumedQuota
	other["actual_quota"] = actualQuota
	for _, clamp := range clamps {
		attachQuotaSaturationToOther(other, clamp)
	}
	if billingPending {
		adminInfo, ok := other["admin_info"].(map[string]interface{})
		if !ok || adminInfo == nil {
			adminInfo = make(map[string]interface{})
			other["admin_info"] = adminInfo
		}
		adminInfo["billing_settlement_pending"] = true
	}
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:    task.UserId,
		LogType:   logType,
		Content:   reason,
		ChannelId: task.ChannelId,
		ModelName: taskModelName(task),
		Quota:     logQuota,
		TokenId:   task.PrivateData.TokenId,
		Group:     task.Group,
		Other:     other,
		NodeName:  task.PrivateData.NodeName,
	})
	return poolSettled && quotaPersisted
}

// RecalculateTaskQuotaByTokens 根据实际 token 消耗重新计费（异步差额结算）。
// 当任务成功且返回了 totalTokens 时，根据模型倍率和分组倍率重新计算实际扣费额度，
// 与预扣费的差额进行补扣或退还。支持钱包和订阅计费来源。
func RecalculateTaskQuotaByTokens(ctx context.Context, task *model.Task, totalTokens int) {
	actualQuota, reason, clamp, ok := calculateTaskQuotaByTokens(task, totalTokens)
	if !ok {
		return
	}
	if RecalculateTaskQuota(ctx, task, actualQuota, reason, clamp) {
		_ = AdjustTaskModelQuotaPoolTokens(task, totalTokens)
	}
}

func calculateTaskQuotaByTokens(task *model.Task, totalTokens int) (int, string, *common.QuotaClamp, bool) {
	if task == nil || totalTokens <= 0 {
		return 0, "", nil, false
	}

	modelName := taskModelName(task)

	// 获取模型价格和倍率
	modelRatio, groupRatio, hasSnapshot, canRecalculate := taskBillingTokenRatios(task)
	if hasSnapshot && !canRecalculate {
		return 0, "", nil, false
	}
	if !hasSnapshot {
		var hasRatioSetting bool
		modelRatio, hasRatioSetting, _ = ratio_setting.GetModelRatio(modelName)
		// 只有配置了倍率(非固定价格)时才按 token 重新计费
		if !hasRatioSetting || modelRatio <= 0 {
			return 0, "", nil, false
		}

		// 获取用户和组的倍率信息
		group := task.Group
		if group == "" {
			user, err := model.GetUserById(task.UserId, false)
			if err == nil {
				group = user.Group
			}
		}
		if group == "" {
			return 0, "", nil, false
		}

		groupRatio = ratio_setting.GetGroupRatio(group)
		userGroupRatio, hasUserGroupRatio := ratio_setting.GetGroupGroupRatio(group, group)
		if hasUserGroupRatio {
			groupRatio = userGroupRatio
		}
	}

	// 计算 OtherRatios 乘积（视频折扣、时长等）
	otherMultiplier := 1.0
	if priceData := taskBillingContextPriceData(task.PrivateData.BillingContext); priceData != nil {
		otherMultiplier = priceData.OtherRatioMultiplier()
	}

	// 计算实际应扣费额度（饱和转换，防止溢出成负数）
	actualQuota, clamp := common.QuotaFromFloatChecked(float64(totalTokens) * modelRatio * groupRatio * otherMultiplier)

	reason := fmt.Sprintf("token重算：tokens=%d, modelRatio=%.2f, groupRatio=%.2f, otherMultiplier=%.4f", totalTokens, modelRatio, groupRatio, otherMultiplier)
	return actualQuota, reason, clamp, actualQuota > 0
}

func ApplyPendingTaskSettlement(ctx context.Context, task *model.Task) error {
	if task == nil || task.PrivateData.BillingContext == nil {
		return nil
	}
	bc := task.PrivateData.BillingContext
	if err := applyPendingTaskSubmissionSettlement(ctx, task); err != nil {
		return err
	}
	if bc.SettlementActualTokens > 0 {
		if err := AdjustTaskModelQuotaPoolTokens(task, bc.SettlementActualTokens); err != nil {
			return err
		}
	}
	if bc.SettlementHasActualQuota {
		clamps := make([]*common.QuotaClamp, 0, 1)
		if bc.SettlementQuotaClamp != nil {
			clamps = append(clamps, bc.SettlementQuotaClamp)
		}
		if !RecalculateTaskQuota(ctx, task, bc.SettlementActualQuota, bc.SettlementReason, clamps...) {
			return errors.New("task quota settlement is still pending")
		}
	}
	bc.SettlementHasActualQuota = false
	bc.SettlementActualQuota = 0
	bc.SettlementActualTokens = 0
	bc.SettlementReason = ""
	bc.SettlementQuotaClamp = nil
	task.BillingStatus = model.TaskBillingStatusSettled
	if task.ID <= 0 {
		return nil
	}
	return task.UpdateBillingState()
}

func applyPendingTaskSubmissionSettlement(ctx context.Context, task *model.Task) error {
	if task == nil || task.PrivateData.BillingContext == nil {
		return nil
	}
	bc := task.PrivateData.BillingContext
	if !bc.SubmissionSettlementPending {
		return nil
	}
	actualQuota := bc.SubmissionActualQuota
	usedReservation, reservationRefunded, err := finalizeTaskSubmissionReservation(ctx, task, actualQuota, false)
	if err != nil {
		return err
	}
	if !usedReservation {
		delta := actualQuota - bc.SubmissionPreConsumedQuota
		if reservationRefunded {
			delta = actualQuota
		}
		if delta != 0 {
			requestID := strings.TrimSpace(bc.SubmissionRequestID)
			if requestID == "" {
				requestID = task.TaskID
			}
			adjustment, err := ensureTaskBillingAdjustment(task, "relay-settle:"+requestID, delta, delta)
			if err != nil {
				return err
			}
			if err := model.ApplyBillingAdjustment(adjustment.ID); err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("任务提交结算等待后台重试 task %s: %s", task.TaskID, err.Error()))
			}
		}
	}
	task.Quota = actualQuota
	if err := settleTaskSubmissionModelQuotaPool(task, actualQuota); err != nil {
		return err
	}
	bc.SubmissionActualQuota = 0
	bc.SubmissionSettlementPending = false
	bc.SubmissionSettled = true
	return nil
}

func finalizeTaskSubmissionReservation(ctx context.Context, task *model.Task, actualQuota int, refund bool) (bool, bool, error) {
	if task == nil || task.PrivateData.BillingContext == nil {
		return false, false, nil
	}
	bc := task.PrivateData.BillingContext
	requestID := strings.TrimSpace(bc.SubmissionRequestID)
	if requestID == "" {
		return false, false, nil
	}
	adjustment, found, err := model.GetBillingAdjustmentByKey("relay-billing:" + requestID)
	if err != nil || !found {
		return false, false, err
	}
	if adjustment.UserID != task.UserId || (task.PrivateData.TokenId > 0 && adjustment.TokenID != task.PrivateData.TokenId) {
		return true, false, errors.New("task submission billing reservation does not match task")
	}
	reservationOutcome := adjustment.ReservationOutcome
	if reservationOutcome == "" && bc.SubmissionPreConsumedQuota > 0 && adjustment.FundingDelta == -bc.SubmissionPreConsumedQuota {
		reservationOutcome = model.BillingReservationOutcomeRefund
	}
	if adjustment.Status != model.BillingAdjustmentStatusReserved && refund {
		if reservationOutcome != model.BillingReservationOutcomeRefund {
			return false, false, nil
		}
		if err := model.ApplyBillingAdjustment(adjustment.ID); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("任务提交退款等待后台重试 task %s: %s", task.TaskID, err.Error()))
		}
		return true, true, nil
	}
	if adjustment.Status != model.BillingAdjustmentStatusReserved && reservationOutcome == model.BillingReservationOutcomeRefund {
		return false, true, nil
	}
	tokenBillingEnabled := bc.SubmissionTokenBilling
	if !tokenBillingEnabled && task.PrivateData.TokenId > 0 && adjustment.TokenDelta != 0 {
		tokenBillingEnabled = true
	}
	adjustment, err = model.FinalizeBillingReservation(adjustment.ID, actualQuota, tokenBillingEnabled, refund)
	if err != nil {
		return true, false, err
	}
	if err := model.ApplyBillingAdjustment(adjustment.ID); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("任务提交计费调整等待后台重试 task %s: %s", task.TaskID, err.Error()))
	}
	return true, refund, nil
}

func taskBillingTokenRatios(task *model.Task) (modelRatio float64, groupRatio float64, hasSnapshot bool, canRecalculate bool) {
	if task == nil || task.PrivateData.BillingContext == nil {
		return 0, 0, false, false
	}
	bc := task.PrivateData.BillingContext
	if bc.ModelPrice > 0 && bc.ModelRatio <= 0 {
		return 0, 0, true, false
	}
	if bc.ModelRatio <= 0 || bc.GroupRatio < 0 {
		return 0, 0, true, false
	}
	return bc.ModelRatio, bc.GroupRatio, true, true
}
