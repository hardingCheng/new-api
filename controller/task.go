package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type taskArtifactResponse struct {
	Key        string `json:"key"`
	Type       string `json:"type"`
	MimeType   string `json:"mime_type,omitempty"`
	ContentURL string `json:"content_url"`
}

var (
	taskArtifactKeyPattern           = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~-]{0,127}$`)
	errTaskArtifactPluginUnavailable = errors.New("task artifact plugin unavailable")
	errTaskArtifactPlugin            = errors.New("task artifact plugin error")
)

func GetTask(c *gin.Context) {
	task, exists, err := model.GetByTaskId(c.GetInt("id"), c.Param("key"))
	if err != nil {
		videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to query task")
		return
	}
	if !exists {
		videoProxyError(c, http.StatusNotFound, "invalid_request_error", "Task not found")
		return
	}
	createdAt := task.CreatedAt
	if createdAt == 0 {
		createdAt = task.SubmitTime
	}
	failReason := task.FailReason
	if task.Status == model.TaskStatusSuccess && taskFailReasonIsLegacyResultURL(task.FailReason) {
		failReason = ""
	}
	c.JSON(http.StatusOK, gin.H{
		"task_id":     task.TaskID,
		"platform":    task.Platform,
		"status":      task.Status,
		"progress":    task.Progress,
		"fail_reason": failReason,
		"created_at":  createdAt,
		"finished_at": task.FinishTime,
	})
}

func GetTaskArtifacts(c *gin.Context) {
	task, exists, err := model.GetByTaskId(c.GetInt("id"), c.Param("key"))
	if err != nil {
		writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_internal_error", "Failed to query task")
		return
	}
	if !exists || task == nil {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return
	}
	writeTaskArtifacts(c, task, false)
}

func GetDashboardTaskArtifacts(c *gin.Context) {
	task, exists, err := getTaskForArtifactRequest(c, c.Param("task_id"))
	if err != nil {
		writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_internal_error", "Failed to query task")
		return
	}
	if !exists || task == nil {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return
	}
	writeTaskArtifacts(c, task, true)
}

func writeTaskArtifacts(c *gin.Context, task *model.Task, dashboard bool) {
	c.Header("Cache-Control", "private, no-store")
	artifacts, err := projectTaskArtifacts(task)
	if err != nil {
		writeTaskArtifactProjectionError(c, err)
		return
	}
	items := make([]taskArtifactResponse, 0, len(artifacts))
	for _, artifact := range artifacts {
		contentURL, buildErr := service.BuildTaskArtifactContentURL(task.TaskID, artifact.Key)
		if buildErr != nil {
			writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_url_error", "Failed to build artifact content URL")
			return
		}
		items = append(items, taskArtifactResponse{
			Key:        artifact.Key,
			Type:       artifact.Type,
			MimeType:   artifact.MimeType,
			ContentURL: contentURL,
		})
	}
	response := gin.H{"task_id": task.TaskID, "artifacts": items}
	if dashboard && task.Status == model.TaskStatusSuccess && task.Platform == constant.TaskPlatformSuno {
		response["legacy_audio_clips"] = legacySunoAudioClips(task.Data)
	}
	if legacyVideoAvailable(task) {
		legacyContentURL, buildErr := service.BuildTaskArtifactContentURL(task.TaskID, "video")
		if buildErr != nil {
			writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_url_error", "Failed to build artifact content URL")
			return
		}
		response["legacy_content_url"] = legacyContentURL
	}
	if dashboard {
		common.ApiSuccess(c, response)
		return
	}
	c.JSON(http.StatusOK, response)
}

// legacySunoAudioClips projects a pre-plugin Suno task's persisted snapshot
// into the clips the dashboard audio preview renders. Task lists no longer
// carry the snapshot, so the dashboard reads it here on demand. The snapshot
// is either a clip array or a JSON string holding one; only clips with an
// audio URL are kept and only preview fields are exposed.
func legacySunoAudioClips(data json.RawMessage) []map[string]any {
	clips := make([]map[string]any, 0)
	if len(data) == 0 {
		return clips
	}
	var items []map[string]any
	if err := common.Unmarshal(data, &items); err != nil {
		var encoded string
		if common.Unmarshal(data, &encoded) != nil || common.UnmarshalJsonStr(encoded, &items) != nil {
			return clips
		}
	}
	for _, item := range items {
		audioURL, _ := item["audio_url"].(string)
		if strings.TrimSpace(audioURL) == "" {
			continue
		}
		clip := map[string]any{"audio_url": audioURL}
		for _, key := range []string{"clip_id", "id", "title", "tags", "duration", "image_url", "image_large_url", "metadata"} {
			if value, ok := item[key]; ok {
				clip[key] = value
			}
		}
		clips = append(clips, clip)
	}
	return clips
}

func projectTaskArtifacts(task *model.Task) ([]relaychannel.TaskArtifact, error) {
	if task == nil || task.Status != model.TaskStatusSuccess || !taskHasPluginExecution(task) || !task.ResultRetrievable() {
		return []relaychannel.TaskArtifact{}, nil
	}
	adaptor := relay.GetTaskAdaptor(task.Platform)
	if adaptor == nil {
		return nil, errTaskArtifactPluginUnavailable
	}
	provider, ok := adaptor.(relaychannel.TaskArtifactProvider)
	if !ok {
		return []relaychannel.TaskArtifact{}, nil
	}
	artifacts, err := provider.ListArtifacts(task)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errTaskArtifactPlugin, err)
	}
	return validateProjectedTaskArtifacts(artifacts)
}

func validateProjectedTaskArtifacts(artifacts []relaychannel.TaskArtifact) ([]relaychannel.TaskArtifact, error) {
	if len(artifacts) > 64 {
		return nil, fmt.Errorf("%w: too many artifacts", errTaskArtifactPlugin)
	}
	seen := make(map[string]struct{}, len(artifacts))
	for i := range artifacts {
		if artifacts[i].Key != strings.TrimSpace(artifacts[i].Key) ||
			artifacts[i].Type != strings.TrimSpace(artifacts[i].Type) {
			return nil, fmt.Errorf("%w: invalid artifact identity", errTaskArtifactPlugin)
		}
		if !taskArtifactKeyPattern.MatchString(artifacts[i].Key) {
			return nil, fmt.Errorf("%w: invalid artifact key", errTaskArtifactPlugin)
		}
		if _, exists := seen[artifacts[i].Key]; exists {
			return nil, fmt.Errorf("%w: duplicate artifact key", errTaskArtifactPlugin)
		}
		seen[artifacts[i].Key] = struct{}{}
		switch artifacts[i].Type {
		case "video", "audio", "image", "file":
		default:
			return nil, fmt.Errorf("%w: invalid artifact type", errTaskArtifactPlugin)
		}
		if len(artifacts[i].MimeType) > 255 || strings.ContainsAny(artifacts[i].MimeType, "\r\n") {
			return nil, fmt.Errorf("%w: invalid artifact mime type", errTaskArtifactPlugin)
		}
	}
	return artifacts, nil
}

func initTaskArtifactAdaptor(task *model.Task) (relaychannel.TaskAdaptor, error) {
	if task == nil || !taskHasPluginExecution(task) {
		return nil, errTaskArtifactPluginUnavailable
	}
	channelModel, err := model.CacheGetChannel(task.ChannelId)
	if err != nil {
		return nil, fmt.Errorf("%w: channel unavailable", errTaskArtifactPluginUnavailable)
	}
	adaptor := relay.GetTaskAdaptor(task.Platform)
	if adaptor == nil {
		return nil, errTaskArtifactPluginUnavailable
	}
	pluginKey := task.PrivateData.Key
	if pluginKey == "" {
		pluginKey = channelModel.Key
	}
	baseURL := channelModel.GetBaseURL()
	if baseURL == "" {
		baseURL = constant.GetChannelBaseURL(channelModel.Type)
	}
	adaptor.Init(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    channelModel.Type,
			ChannelBaseUrl: baseURL,
			ApiKey:         pluginKey,
			ChannelSetting: channelModel.GetSetting(),
		},
	})
	return adaptor, nil
}

func taskHasPluginExecution(task *model.Task) bool {
	return task != nil &&
		task.PrivateData.Execution != nil &&
		task.PrivateData.Execution.TaskPlugin != nil &&
		strings.TrimSpace(task.PrivateData.Execution.TaskPlugin.Key) != ""
}

func legacyVideoAvailable(task *model.Task) bool {
	if task == nil || task.Status != model.TaskStatusSuccess ||
		taskHasPluginExecution(task) || task.Platform == constant.TaskPlatformSuno ||
		strings.TrimSpace(task.GetResultURL()) == "" {
		return false
	}
	switch constant.NormalizeTaskAction(task.Action) {
	case constant.TaskActionImageToVideo,
		constant.TaskActionTextToVideo,
		constant.TaskActionFirstTailToVideo,
		constant.TaskActionReferenceToVideo,
		constant.TaskActionRemix,
		constant.TaskActionAvatarGenerate,
		constant.TaskActionEnhance:
		return true
	default:
		return false
	}
}

func getTaskForArtifactRequest(c *gin.Context, taskID string) (*model.Task, bool, error) {
	if middleware.IsTaskArtifactAccess(c) {
		task, exists, err := model.GetUniqueByOnlyTaskId(taskID)
		if err != nil || !exists || task == nil {
			return task, exists, err
		}
		owner, err := model.GetUserCache(task.UserId)
		if err != nil || owner == nil || owner.Status != common.UserStatusEnabled {
			return nil, false, err
		}
		return task, true, nil
	}
	if c.GetInt("token_id") == 0 && c.GetInt("role") >= common.RoleAdminUser {
		return model.GetByOnlyTaskId(taskID)
	}
	return model.GetByTaskId(c.GetInt("id"), taskID)
}

func writeTaskArtifactProjectionError(c *gin.Context, err error) {
	if errors.Is(err, errTaskArtifactPluginUnavailable) {
		writeTaskArtifactError(c, http.StatusServiceUnavailable, "artifact_plugin_unavailable", "Artifact preview plugin is unavailable")
		return
	}
	writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_plugin_error", "Artifact preview plugin failed")
}

func writeTaskArtifactError(c *gin.Context, status int, code, message string) {
	c.Header("Cache-Control", "private, no-store")
	if middleware.IsTaskArtifactAccess(c) {
		status = http.StatusNotFound
		code = "artifact_not_found"
		message = "Task or artifact not found"
	}
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		c.JSON(status, gin.H{"success": false, "code": code, "message": message})
		return
	}
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    code,
			"code":    code,
		},
	})
}

func TaskArtifactContent(c *gin.Context) {
	task, exists, err := getTaskForArtifactRequest(c, c.Param("key"))
	if err != nil {
		writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_internal_error", "Failed to query task")
		return
	}
	if !exists || task == nil {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return
	}
	artifactKey := strings.TrimSpace(c.Param("artifact_key"))
	if !taskArtifactKeyPattern.MatchString(artifactKey) || !task.ResultRetrievable() {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return
	}
	if task.Status != model.TaskStatusSuccess {
		writeTaskArtifactError(c, http.StatusConflict, "artifact_not_ready", "Task artifacts are not ready")
		return
	}
	if !taskHasPluginExecution(task) {
		if artifactKey != "video" || !legacyVideoAvailable(task) {
			writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
			return
		}
		descriptor := &relaychannel.TaskContentRequest{
			URL:            task.GetResultURL(),
			Method:         c.Request.Method,
			Credentialless: true,
		}
		if err := proxyTaskMedia(c, task, descriptor); err != nil {
			writeTaskMediaProxyError(c, err)
		}
		return
	}
	artifacts, err := projectTaskArtifacts(task)
	if err != nil {
		writeTaskArtifactProjectionError(c, err)
		return
	}
	found := false
	for _, artifact := range artifacts {
		if artifact.Key == artifactKey {
			found = true
			break
		}
	}
	if !found {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return
	}
	artifactStore := service.GetTaskArtifactStore()
	if ref, resolveErr := artifactStore.Resolve(task, artifactKey); resolveErr == nil && ref != nil {
		_ = artifactStore.Serve(c, task, ref)
		return
	}

	adaptor, err := initTaskArtifactAdaptor(task)
	if err != nil {
		writeTaskArtifactProjectionError(c, err)
		return
	}
	provider, ok := adaptor.(relaychannel.TaskContentRequestProvider)
	if !ok {
		writeTaskArtifactError(c, http.StatusServiceUnavailable, "artifact_plugin_unavailable", "Artifact content plugin is unavailable")
		return
	}
	clientRequest := relaychannel.TaskArtifactClientRequest{
		Method:  c.Request.Method,
		Headers: taskArtifactClientHeaders(c.Request.Header),
	}
	descriptor, err := provider.BuildContentRequest(task, artifactKey, clientRequest)
	if err != nil || descriptor == nil {
		writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_plugin_error", "Artifact content plugin failed")
		return
	}
	if err := proxyTaskMedia(c, task, descriptor); err != nil {
		writeTaskMediaProxyError(c, err)
	}
}

func taskArtifactClientHeaders(headers http.Header) map[string]string {
	result := make(map[string]string, 4)
	for _, name := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			result[name] = value
		}
	}
	return result
}

/*
	The task list handlers below deliberately do not call projectTaskArtifacts.
	Artifact projection is confined to the explicit endpoints above.
*/

func GetAllTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// 解析其他查询参数
	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		ModelName:      strings.TrimSpace(c.Query("model_name")),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ChannelID:      c.Query("channel_id"),
	}
	if username := strings.TrimSpace(c.Query("username")); username != "" {
		userIDs, err := model.SearchUserIDsByUsername(username, 1000)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		queryParams.UserIDs = userIDs
		if len(userIDs) == 0 {
			pageInfo.SetTotal(0)
			pageInfo.SetItems([]*dto.TaskDto{})
			common.ApiSuccess(c, pageInfo)
			return
		}
	}

	items := model.TaskGetAllTasks(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllTasks(queryParams)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tasksToDto(items, taskDtoOptions{
		adminView:             true,
		includeUpstreamTaskID: true,
		viewerRole:            c.GetInt("role"),
	}))
	common.ApiSuccess(c, pageInfo)
}

func GetUserTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	userId := c.GetInt("id")

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
	}

	items := model.TaskGetAllUserTask(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllUserTask(userId, queryParams)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tasksToDto(items, taskDtoOptions{viewerRole: common.RoleCommonUser}))
	common.ApiSuccess(c, pageInfo)
}

const (
	taskExportMaxRangeSeconds = int64(31 * 24 * 60 * 60)
	taskExportMaxRows         = 5000
)

// GetAllTaskExport 返回指定时间范围内的任务，供后台导出报表使用。
func GetAllTaskExport(c *gin.Context) {
	startTimestamp, err := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	if err != nil || startTimestamp <= 0 {
		common.ApiErrorMsg(c, "invalid start_timestamp")
		return
	}
	endTimestamp, err := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if err != nil || endTimestamp <= 0 {
		common.ApiErrorMsg(c, "invalid end_timestamp")
		return
	}
	if endTimestamp < startTimestamp {
		common.ApiErrorMsg(c, "invalid time range")
		return
	}
	if endTimestamp-startTimestamp > taskExportMaxRangeSeconds {
		common.ApiErrorMsg(c, "task export time range cannot exceed 31 days")
		return
	}
	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		ModelName:      strings.TrimSpace(c.Query("model_name")),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ChannelID:      c.Query("channel_id"),
	}
	if username := strings.TrimSpace(c.Query("username")); username != "" {
		userIDs, err := model.SearchUserIDsByUsername(username, 1000)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		queryParams.UserIDs = userIDs
		if len(userIDs) == 0 {
			common.ApiSuccess(c, gin.H{"items": []*dto.TaskDto{}})
			return
		}
	}
	items, err := model.TaskGetAllTasksForExport(taskExportMaxRows+1, queryParams)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(items) > taskExportMaxRows {
		common.ApiErrorMsg(c, "task export exceeds 5000 rows; narrow the filters")
		return
	}
	common.ApiSuccess(c, gin.H{"items": tasksToDto(items, taskDtoOptions{
		adminView:  true,
		viewerRole: c.GetInt("role"),
	})})
}

func GetModelQuotaPoolUsage(c *gin.Context) {
	userID := c.GetInt("id")
	includeAllUserPools := c.GetInt("role") >= common.RoleAdminUser && strings.TrimSpace(c.Query("scope")) != "self"
	common.ApiSuccess(c, service.GetVisibleModelQuotaPoolUsage(userID, includeAllUserPools))
}

type taskDtoOptions struct {
	adminView             bool
	includeUpstreamTaskID bool
	viewerRole            int
}

func tasksToDto(tasks []*model.Task, options taskDtoOptions) []*dto.TaskDto {
	var userIdMap map[int]*model.UserBase
	channelIdMap := make(map[int]string)
	if options.adminView {
		userIdMap = make(map[int]*model.UserBase)
		userIds := types.NewSet[int]()
		for _, task := range tasks {
			if task.UserId > 0 {
				userIds.Add(task.UserId)
			}
		}
		for _, userId := range userIds.Items() {
			cacheUser, err := model.GetUserCache(userId)
			if err == nil {
				userIdMap[userId] = cacheUser
			}
		}
	}
	channelIds := types.NewSet[int]()
	for _, task := range tasks {
		if task.ChannelId != 0 {
			channelIds.Add(task.ChannelId)
		}
	}
	for _, channelId := range channelIds.Items() {
		channel, err := model.CacheGetChannel(channelId)
		if err == nil && channel != nil {
			channelIdMap[channelId] = channel.Name
		}
	}
	result := make([]*dto.TaskDto, len(tasks))
	for i, task := range tasks {
		if options.adminView {
			if user, ok := userIdMap[task.UserId]; ok {
				task.Username = user.Username
			}
		}
		if channelName, ok := channelIdMap[task.ChannelId]; ok {
			task.ChannelName = channelName
		}
		dtoItem := relay.TaskModel2Dto(task)
		if options.includeUpstreamTaskID {
			dtoItem.UpstreamTaskID = task.GetUpstreamTaskID()
		}
		dtoItem.LegacyVideoAvailable = legacyVideoAvailable(task)
		dtoItem.ResultDiscarded = task.PrivateData.ResultDiscarded
		if task.Status == model.TaskStatusSuccess && taskFailReasonIsLegacyResultURL(task.FailReason) {
			dtoItem.FailReason = ""
		}
		appendTaskDtoRoleInfo(dtoItem, task, options.viewerRole)
		// 普通用户路径（非管理员）脱敏：移除计费/渠道/上游模型名等内部字段
		if !options.adminView {
			redactTaskDtoForUser(dtoItem)
		}
		result[i] = dtoItem
	}
	return result
}

// redactTaskDtoForUser 移除普通用户不应看到的内部字段：
// 计费额度、归属渠道/分组、内部主键，以及上游真实模型名和上游内部 task_id。
func redactTaskDtoForUser(d *dto.TaskDto) {
	d.Quota = 0
	d.RefundQuota = 0
	// 上游原始结果地址常带签名，用户侧一律走 /content 代理，不下发原始地址
	d.ResultURL = ""
	d.URL = ""
	d.VideoURL = ""
	d.UpstreamTaskID = ""
	// 注意：d.Key 是任务的数据库自增 ID 字符串（非 API 密钥），
	// 前端任务表格用它作为 rowKey，置空会导致行 key 冲突，故保留。
	d.Group = ""
	d.ChannelId = 0
	d.ChannelName = ""
	publicModel := ""
	if props, ok := d.Properties.(model.Properties); ok {
		publicModel = strings.TrimSpace(props.OriginModelName)
		props.UpstreamModelName = ""
		d.Properties = props
	}
	d.ModelName = publicModel
	d.Data = stripTaskDataResultURLs(relay.RedactTaskDataForPublic(d.Data, publicModel, d.TaskID))
}

// stripTaskDataResultURLs 去掉 data 顶层注入的结果地址（上游原始地址常带签名），其余字段原样保留。
func stripTaskDataResultURLs(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(data, &fields); err != nil {
		return data
	}
	changed := false
	for _, key := range []string{"result_url", "url", "video_url"} {
		if _, ok := fields[key]; ok {
			delete(fields, key)
			changed = true
		}
	}
	if !changed {
		return data
	}
	encoded, err := common.Marshal(fields)
	if err != nil {
		return data
	}
	return encoded
}

// appendTaskDtoRoleInfo 按查看者角色附加插件执行信息（管理员）与上游标识（root）。
func appendTaskDtoRoleInfo(item *dto.TaskDto, task *model.Task, viewerRole int) {
	if viewerRole >= common.RoleAdminUser {
		adminInfo := &dto.TaskAdminInfo{}
		if execution := task.PrivateData.Execution; execution != nil {
			adminInfo.RequestID = execution.RequestID
			adminInfo.RequestPath = execution.RequestPath
			if snapshot := execution.TaskPlugin; snapshot != nil {
				adminInfo.TaskPlugin = &dto.TaskPluginInfo{
					Key:     snapshot.Key,
					Name:    snapshot.Name,
					Version: snapshot.Version,
				}
				if snapshot.Author != nil {
					adminInfo.TaskPlugin.Author = &dto.TaskPluginAuthorInfo{
						Name: snapshot.Author.Name,
						URL:  snapshot.Author.URL,
					}
				}
			}
		}
		if adminInfo.RequestID != "" || adminInfo.RequestPath != "" || adminInfo.TaskPlugin != nil {
			item.AdminInfo = adminInfo
		}
	}
	if viewerRole >= common.RoleRootUser {
		rootInfo := &dto.TaskRootInfo{
			UpstreamTaskID: task.PrivateData.UpstreamTaskID,
			NodeName:       task.PrivateData.NodeName,
		}
		if execution := task.PrivateData.Execution; execution != nil {
			if snapshot := execution.TaskPlugin; snapshot != nil {
				rootInfo.TaskPlugin = &dto.TaskPluginRuntimeInfo{
					Key:        snapshot.Key,
					Version:    snapshot.Version,
					APIVersion: snapshot.APIVersion,
					Generation: snapshot.Generation,
				}
			}
		}
		if rootInfo.TaskPlugin != nil || rootInfo.UpstreamTaskID != "" || rootInfo.NodeName != "" {
			item.RootInfo = rootInfo
		}
	}
}

func taskFailReasonIsLegacyResultURL(value string) bool {
	value = strings.TrimSpace(value)
	return len(value) >= len("https://") && strings.EqualFold(value[:len("https://")], "https://") ||
		len(value) >= len("http://") && strings.EqualFold(value[:len("http://")], "http://") ||
		len(value) >= len("data:") && strings.EqualFold(value[:len("data:")], "data:")
}
