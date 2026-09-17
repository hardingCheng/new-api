package tencentmps

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

const sourceURL = "https://example.com/videos/in.mp4"

func TestParseModelName(t *testing.T) {
	cases := map[string]modelSpec{
		"video-enhance-720p":  {Family: familyEnhance, Resolution: "720p", ShortEdge: 720},
		"video-enhance-1080p": {Family: familyEnhance, Resolution: "1080p", ShortEdge: 1080},
		"Video-Enhance-2K":    {Family: familyEnhance, Resolution: "2k", ShortEdge: 1440},
		"video-enhance-4k":    {Family: familyEnhance, Resolution: "4k", ShortEdge: 2160},
		"video-restore-720p":  {Family: familyRestore, Resolution: "720p", ShortEdge: 720},
		" video-restore-4k ":  {Family: familyRestore, Resolution: "4k", ShortEdge: 2160},
	}
	for name, want := range cases {
		got, ok := parseModelName(name)
		if !ok || got != want {
			t.Fatalf("parseModelName(%q) = %+v, %v; want %+v", name, got, ok, want)
		}
	}
	for _, bad := range []string{"", "video-enhance", "video-enhance-480p", "video-enhance-8k", "seedance-1.0", "video-restore-"} {
		if _, ok := parseModelName(bad); ok {
			t.Fatalf("parseModelName(%q) should fail", bad)
		}
	}
	for _, name := range ModelList {
		if _, ok := parseModelName(name); !ok {
			t.Fatalf("model list entry %q does not parse", name)
		}
	}
}

func TestPlanFromRequestTemplatesAndScene(t *testing.T) {
	cases := []struct {
		model, scene string
		wantTemplate int
	}{
		{"video-enhance-720p", "", 327001},
		{"video-enhance-1080p", "", 327003},
		{"video-enhance-2k", "real", 327005},
		{"video-enhance-4k", "REAL", 327007},
		{"video-enhance-1080p", "anime", 327004},
		{"video-enhance-720p", "real-small-face", 327025},
		{"video-enhance-4k", "anime-small-face", 327032},
		{"video-enhance-2k", "seedance", 327035},
		{"video-restore-720p", "", 327021},
		{"video-restore-1080p", "anime", 327022}, // 修复系列不分场景
		{"video-restore-4k", "", 327024},
	}
	for _, tc := range cases {
		req := relaycommon.TaskSubmitReq{Model: tc.model, InputReference: sourceURL, Seconds: "30"}
		if tc.scene != "" {
			req.Metadata = map[string]interface{}{"scene": tc.scene}
		}
		plan, err := planFromRequest(&req)
		if err != nil {
			t.Fatalf("%s/%s: %v", tc.model, tc.scene, err)
		}
		if plan.TemplateID != tc.wantTemplate {
			t.Fatalf("%s/%s: template = %d, want %d", tc.model, tc.scene, plan.TemplateID, tc.wantTemplate)
		}
		if plan.Seconds != 30 || plan.SourceURL != sourceURL {
			t.Fatalf("%s: plan = %+v", tc.model, plan)
		}
	}
}

func TestPlanFromRequestRejectsBadInput(t *testing.T) {
	good := func() relaycommon.TaskSubmitReq {
		return relaycommon.TaskSubmitReq{Model: "video-enhance-1080p", InputReference: sourceURL, Seconds: "30"}
	}

	req := good()
	req.Model = "video-enhance-480p"
	if _, err := planFromRequest(&req); err == nil || !strings.Contains(err.Error(), "unsupported model") {
		t.Fatalf("bad model: %v", err)
	}

	req = good()
	req.InputReference = ""
	if _, err := planFromRequest(&req); err == nil || !strings.Contains(err.Error(), "input_reference") {
		t.Fatalf("missing source: %v", err)
	}

	req = good()
	req.InputReference = "data:video/mp4;base64,AAAA"
	if _, err := planFromRequest(&req); err == nil || !strings.Contains(err.Error(), "http(s)") {
		t.Fatalf("data uri: %v", err)
	}

	req = good()
	req.Seconds = ""
	if _, err := planFromRequest(&req); err == nil || !strings.Contains(err.Error(), "seconds") {
		t.Fatalf("missing seconds: %v", err)
	}

	req = good()
	req.Seconds = "-1"
	req.Duration = -1
	if _, err := planFromRequest(&req); err == nil || !strings.Contains(err.Error(), "seconds") {
		t.Fatalf("auto duration: %v", err)
	}

	req = good()
	req.Seconds = "3601"
	if _, err := planFromRequest(&req); err == nil || !strings.Contains(err.Error(), "3600") {
		t.Fatalf("too long: %v", err)
	}

	req = good()
	req.Metadata = map[string]interface{}{"scene": "cartoon"}
	if _, err := planFromRequest(&req); err == nil || !strings.Contains(err.Error(), "unsupported scene") {
		t.Fatalf("bad scene: %v", err)
	}
}

func TestSourceVideoURLFallbacks(t *testing.T) {
	req := relaycommon.TaskSubmitReq{Metadata: map[string]interface{}{"video_url": " https://a/b.mp4 "}}
	if got := sourceVideoURL(&req); got != "https://a/b.mp4" {
		t.Fatalf("metadata.video_url = %q", got)
	}
	req = relaycommon.TaskSubmitReq{Content: []relaycommon.TaskContentItem{
		{Type: "text", Text: "x"},
		{Type: "video_url", VideoURL: &relaycommon.TaskMediaURL{URL: "https://a/c.mp4"}},
	}}
	if got := sourceVideoURL(&req); got != "https://a/c.mp4" {
		t.Fatalf("content video_url = %q", got)
	}
	// duration 字段与 seconds 字段取较大者
	req = relaycommon.TaskSubmitReq{Model: "video-enhance-720p", InputReference: sourceURL, Duration: 45}
	plan, err := planFromRequest(&req)
	if err != nil || plan.Seconds != 45 {
		t.Fatalf("duration field: %+v, %v", plan, err)
	}
}

func TestBuildProcessMediaPayload(t *testing.T) {
	req := relaycommon.TaskSubmitReq{Model: "video-enhance-1080p", InputReference: sourceURL, Seconds: "30"}
	plan, err := planFromRequest(&req)
	if err != nil {
		t.Fatal(err)
	}
	cred := credentials{SecretID: "id", SecretKey: "sk", Region: "ap-guangzhou", Bucket: "bkt-1250000000"}
	payload, err := buildProcessMediaPayload(plan, cred)
	if err != nil {
		t.Fatal(err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatal(err)
	}
	input := body["InputInfo"].(map[string]interface{})
	if input["Type"] != "URL" || input["UrlInputInfo"].(map[string]interface{})["Url"] != sourceURL {
		t.Fatalf("InputInfo = %v", input)
	}
	storage := body["OutputStorage"].(map[string]interface{})
	cos := storage["CosOutputStorage"].(map[string]interface{})
	if storage["Type"] != "COS" || cos["Bucket"] != "bkt-1250000000" || cos["Region"] != "ap-guangzhou" {
		t.Fatalf("OutputStorage = %v", storage)
	}
	tasks := body["MediaProcessTask"].(map[string]interface{})["TranscodeTaskSet"].([]interface{})
	if len(tasks) != 1 {
		t.Fatalf("TranscodeTaskSet = %v", tasks)
	}
	task := tasks[0].(map[string]interface{})
	if task["Definition"].(float64) != 327003 {
		t.Fatalf("Definition = %v", task["Definition"])
	}
	ext := task["OverrideParameter"].(map[string]interface{})["StdExtInfo"].(string)
	var extObj struct {
		VideoInfo      struct{ Short int } `json:"video_info"`
		UseInternalCos int                 `json:"use_internal_cos"`
	}
	if err := json.Unmarshal([]byte(ext), &extObj); err != nil {
		t.Fatalf("StdExtInfo is not a JSON string: %q", ext)
	}
	if extObj.VideoInfo.Short != 1080 || extObj.UseInternalCos != 1 {
		t.Fatalf("StdExtInfo = %q", ext)
	}
	// 密钥绝不能出现在请求体里
	if strings.Contains(string(payload), "sk") && strings.Contains(string(payload), `"sk"`) {
		t.Fatalf("secret leaked into payload: %s", payload)
	}
}

func TestParseSubmitResponse(t *testing.T) {
	id, apiErr, err := parseSubmitResponse([]byte(`{"Response":{"RequestId":"r","TaskId":"2600005051-WorkflowTask-abc"}}`))
	if err != nil || apiErr != nil || id != "2600005051-WorkflowTask-abc" {
		t.Fatalf("ok response: id=%q apiErr=%v err=%v", id, apiErr, err)
	}
	_, apiErr, err = parseSubmitResponse([]byte(`{"Response":{"RequestId":"r","Error":{"Code":"AuthFailure.SignatureFailure","Message":"bad sig"}}}`))
	if err != nil || apiErr == nil || apiErr.Code != "AuthFailure.SignatureFailure" {
		t.Fatalf("error response: apiErr=%v err=%v", apiErr, err)
	}
	if _, _, err = parseSubmitResponse([]byte(`{"Response":{"RequestId":"r"}}`)); err == nil {
		t.Fatal("missing task id should fail")
	}
	if _, _, err = parseSubmitResponse([]byte(`not json`)); err == nil {
		t.Fatal("garbage should fail")
	}
}

func TestStatusForErrorCode(t *testing.T) {
	cases := map[string]int{
		"InvalidParameterValue.InvalidInputUrl": http.StatusBadRequest,
		"MissingParameter":                      http.StatusBadRequest,
		"AuthFailure.SecretIdNotFound":          http.StatusUnauthorized,
		"UnauthorizedOperation":                 http.StatusUnauthorized,
		"RequestLimitExceeded":                  http.StatusTooManyRequests,
		"LimitExceeded.TooMuchTask":             http.StatusTooManyRequests,
		"InternalError":                         http.StatusBadGateway,
		"FailedOperation.InsufficientBalance":   http.StatusBadGateway,
	}
	for code, want := range cases {
		if got := statusForErrorCode(code); got != want {
			t.Fatalf("statusForErrorCode(%q) = %d, want %d", code, got, want)
		}
	}
}

const workflowFinished = `{"Response":{"RequestId":"r","Status":"FINISH","TaskType":"WorkflowTask","WorkflowTask":{
  "Status":"FINISH","ErrCode":0,"Message":"SUCCESS",
  "MediaProcessResultSet":[{"Type":"Transcode","TranscodeTask":{"Status":"SUCCESS","ErrCode":0,"Message":"SUCCESS","Progress":100,
    "Output":{"Path":"/out/a.mp4","Duration":120.4,"Width":1920,"Height":1080,
      "VideoStreamSet":[{"Fps":60,"Codec":"h264"}],
      "OutputStorage":{"Type":"COS","CosOutputStorage":{"Bucket":"bkt-1250000000","Region":"ap-shanghai"}},
      "CallBackExtInfo":"{\"tmpl_url\":\"https://vod-trans.cos.ap-shanghai.myqcloud.com/for_online_user/a.mp4\"}"}}}]}}}`

const scheduleFinished = `{"Response":{"RequestId":"r","Status":"FINISH","TaskType":"ScheduleTask","ScheduleTask":{
  "Status":"FINISH","ErrCode":0,"Message":"SUCCESS",
  "ActivityResultSet":[{"ActivityType":"Transcode","ActivityResItem":{"TranscodeTask":{"Status":"SUCCESS","ErrCode":0,"Message":"SUCCESS","Progress":100,
    "Output":{"Path":"/out/b.mp4","Duration":20.08,"VideoStreamSet":[{"Fps":30}],
      "CallBackExtInfo":"{\"tmpl_url\":\"https://vod-trans.cos.ap-shanghai.myqcloud.com/for_online_user/b.mp4\"}"}}}}]}}}`

func TestParseTaskResultStates(t *testing.T) {
	a := &TaskAdaptor{}

	info, err := a.ParseTaskResult([]byte(`{"Response":{"RequestId":"r","Status":"WAITING","WorkflowTask":{"Status":"WAITING"}}}`))
	if err != nil || info.Status != model.TaskStatusQueued {
		t.Fatalf("waiting: %+v, %v", info, err)
	}

	info, err = a.ParseTaskResult([]byte(`{"Response":{"RequestId":"r","Status":"PROCESSING","WorkflowTask":{"Status":"PROCESSING",
	  "MediaProcessResultSet":[{"Type":"Transcode","TranscodeTask":{"Status":"PROCESSING","Progress":100}}]}}}`))
	if err != nil || info.Status != model.TaskStatusInProgress || info.Progress != "99%" {
		t.Fatalf("processing should cap progress at 99%%: %+v, %v", info, err)
	}

	info, err = a.ParseTaskResult([]byte(`{"Response":{"RequestId":"r","Status":"PROCESSING","WorkflowTask":{"Status":"PROCESSING",
	  "MediaProcessResultSet":[{"Type":"Transcode","TranscodeTask":{"Status":"PROCESSING","Progress":42}}]}}}`))
	if err != nil || info.Progress != "42%" {
		t.Fatalf("processing progress: %+v, %v", info, err)
	}

	info, err = a.ParseTaskResult([]byte(workflowFinished))
	if err != nil || info.Status != model.TaskStatusSuccess || info.Progress != "100%" {
		t.Fatalf("workflow success: %+v, %v", info, err)
	}
	if info.Url != "https://vod-trans.cos.ap-shanghai.myqcloud.com/for_online_user/a.mp4" {
		t.Fatalf("workflow url = %q", info.Url)
	}

	info, err = a.ParseTaskResult([]byte(scheduleFinished))
	if err != nil || info.Status != model.TaskStatusSuccess || info.Url != "https://vod-trans.cos.ap-shanghai.myqcloud.com/for_online_user/b.mp4" {
		t.Fatalf("schedule success: %+v, %v", info, err)
	}

	info, err = a.ParseTaskResult([]byte(`{"Response":{"RequestId":"r","Status":"FINISH","WorkflowTask":{"Status":"FINISH","ErrCode":0,"Message":"SUCCESS",
	  "MediaProcessResultSet":[{"Type":"Transcode","TranscodeTask":{"Status":"FAIL","ErrCode":40000,"ErrCodeExt":"InvalidInput","Message":"input video is corrupted"}}]}}}`))
	if err != nil || info.Status != model.TaskStatusFailure || info.Reason != "input video is corrupted" {
		t.Fatalf("workflow fail: %+v, %v", info, err)
	}

	info, err = a.ParseTaskResult([]byte(`{"Response":{"RequestId":"r","Status":"FINISH","WorkflowTask":{"Status":"FINISH","ErrCode":0,"Message":"SUCCESS",
	  "MediaProcessResultSet":[{"Type":"Transcode","TranscodeTask":{"Status":"SUCCESS","ErrCode":0,"Message":"SUCCESS","Output":{"Path":"/x.mp4"}}}]}}}`))
	if err != nil || info.Status != model.TaskStatusFailure || !strings.Contains(info.Reason, "no output url") {
		t.Fatalf("success without url must fail: %+v, %v", info, err)
	}

	info, err = a.ParseTaskResult([]byte(`{"Response":{"RequestId":"r","Error":{"Code":"ResourceNotFound.TaskNotFound","Message":"gone"}}}`))
	if err != nil || info.Status != model.TaskStatusFailure || !strings.Contains(info.Reason, "ResourceNotFound") {
		t.Fatalf("permanent error: %+v, %v", info, err)
	}

	if _, err = a.ParseTaskResult([]byte(`{"Response":{"RequestId":"r","Error":{"Code":"AuthFailure.SignatureExpire","Message":"expired"}}}`)); err == nil {
		t.Fatal("transient error must be returned for retry, not mark the task failed")
	}
}

func TestOutputURLFallsBackToBucketPath(t *testing.T) {
	out := &transcodeOutput{
		Path:          "/out/a.mp4",
		OutputStorage: &outputStorage{Type: "COS", CosOutputStorage: &cosOutputStorage{Bucket: "bkt-1250000000", Region: "ap-shanghai"}},
	}
	if got := outputURL(out); got != "https://bkt-1250000000.cos.ap-shanghai.myqcloud.com/out/a.mp4" {
		t.Fatalf("bucket url = %q", got)
	}
	out.CallBackExtInfo = `{"tmpl_url":"https://tmp/a.mp4"}`
	if got := outputURL(out); got != "https://tmp/a.mp4" {
		t.Fatalf("tmpl_url should win: %q", got)
	}
	if got := outputURL(nil); got != "" {
		t.Fatalf("nil output = %q", got)
	}
}

func TestFpsBillingRatio(t *testing.T) {
	cases := map[float64]float64{0: 1, 23.976: 1, 24: 1, 25: 1, 29.97: 1, 30: 1, 50: 2, 59.94: 2, 60: 2, 90: 4, 120: 4, 240: 4}
	for fps, want := range cases {
		if got := fpsBillingRatio(fps); got != want {
			t.Fatalf("fpsBillingRatio(%v) = %v, want %v", fps, got, want)
		}
	}
}

func TestAdjustBillingOnCompleteRescalesByActualOutput(t *testing.T) {
	a := &TaskAdaptor{}
	success := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}
	newTask := func(data string, quota int, estSeconds float64) *model.Task {
		task := &model.Task{Quota: quota, Data: json.RawMessage(data)}
		task.PrivateData.BillingContext = &model.TaskBillingContext{OtherRatios: map[string]float64{"seconds": estSeconds, "fps": 1}}
		return task
	}

	// 声明 100 秒、按 ≤30 帧预扣 1000；实际输出 120.4 秒（进位 121）、60 帧 → 1000 × 121 × 2 / 100
	if got := a.AdjustBillingOnComplete(newTask(workflowFinished, 1000, 100), success); got != 2420 {
		t.Fatalf("60fps longer output: got %d, want 2420", got)
	}
	// 声明 30 秒、实际 20.08 秒（进位 21）、30 帧 → 退差额：1000 × 21 / 30 = 700
	if got := a.AdjustBillingOnComplete(newTask(scheduleFinished, 1000, 30), success); got != 700 {
		t.Fatalf("shorter output: got %d, want 700", got)
	}
	// 声明与实际一致 → 不结算
	if got := a.AdjustBillingOnComplete(newTask(scheduleFinished, 1000, 21), success); got != 0 {
		t.Fatalf("exact estimate should keep pre-charge, got %d", got)
	}
	// 失败任务 / 没有计费上下文 / 没有输出 → 不结算
	if got := a.AdjustBillingOnComplete(newTask(workflowFinished, 1000, 100), &relaycommon.TaskInfo{Status: model.TaskStatusFailure}); got != 0 {
		t.Fatalf("failure should not settle, got %d", got)
	}
	task := newTask(workflowFinished, 1000, 100)
	task.PrivateData.BillingContext = nil
	if got := a.AdjustBillingOnComplete(task, success); got != 0 {
		t.Fatalf("missing billing context should not settle, got %d", got)
	}
	if got := a.AdjustBillingOnComplete(newTask(`{"Response":{"Status":"FINISH"}}`, 1000, 100), success); got != 0 {
		t.Fatalf("missing output should not settle, got %d", got)
	}
	// 上下文没有 seconds 时退回提交时记录的时长
	task = newTask(scheduleFinished, 1000, 0)
	task.Properties.VideoSeconds = 30
	if got := a.AdjustBillingOnComplete(task, success); got != 700 {
		t.Fatalf("fallback to VideoSeconds: got %d, want 700", got)
	}
}

func TestConvertToOpenAIVideo(t *testing.T) {
	a := &TaskAdaptor{}
	task := &model.Task{TaskID: "task_1", Status: model.TaskStatusSuccess, Progress: "100%", Data: json.RawMessage(workflowFinished)}
	task.Properties.OriginModelName = "video-enhance-1080p"
	out, err := a.ConvertToOpenAIVideo(task)
	if err != nil {
		t.Fatal(err)
	}
	var video map[string]interface{}
	if err := json.Unmarshal(out, &video); err != nil {
		t.Fatal(err)
	}
	if video["id"] != "task_1" || video["model"] != "video-enhance-1080p" {
		t.Fatalf("video = %v", video)
	}
	meta, _ := video["metadata"].(map[string]interface{})
	if meta["url"] != "https://vod-trans.cos.ap-shanghai.myqcloud.com/for_online_user/a.mp4" {
		t.Fatalf("metadata = %v", meta)
	}
}

// 2026-09-17 直连上游真跑一单(720p 24 帧 5.06 秒 → 1080p 真人模板)拿到的 DescribeTaskDetail 终态响应,
// 已脱敏。hub 对上游形状的全部假设都要能在这份真样本上成立。
func TestRealFinishedResponse(t *testing.T) {
	body, err := os.ReadFile("testdata/describe_task_detail_finish.json")
	if err != nil {
		t.Fatal(err)
	}
	a := &TaskAdaptor{}
	info, err := a.ParseTaskResult(body)
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != model.TaskStatusSuccess || info.Progress != "100%" {
		t.Fatalf("status = %+v", info)
	}
	wantURL := "https://vod-trans-1250000001.cos.ap-shanghai.myqcloud.com/for_online_user/20260917/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee-concat/sample_transcode_327003.mp4"
	if info.Url != wantURL {
		t.Fatalf("url = %q", info.Url)
	}

	// 声明 5 秒预扣 1000;实际输出 5.062 秒进位 6 秒、24 帧基准档 → 1200
	task := &model.Task{Quota: 1000, Data: json.RawMessage(body), Status: model.TaskStatusSuccess}
	task.PrivateData.BillingContext = &model.TaskBillingContext{OtherRatios: map[string]float64{"seconds": 5, "fps": 1}}
	if got := a.AdjustBillingOnComplete(task, info); got != 1200 {
		t.Fatalf("settlement = %d, want 1200", got)
	}

	// 对客查询响应能拿到地址
	task.TaskID = "task_real"
	task.Properties.OriginModelName = "video-enhance-1080p"
	out, err := a.ConvertToOpenAIVideo(task)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), wantURL) {
		t.Fatalf("openai video lacks url: %s", out)
	}
}
