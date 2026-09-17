package tencentmps

// 音视频超分增强：把一段已有视频送去云端媒体处理服务做大模型画质增强 / 老片修复，
// 输出更高分辨率的新视频。上游接口是腾讯云 MPS 的 ProcessMedia（提交）+
// DescribeTaskDetail（查询），签名为 TC3-HMAC-SHA256。
//
// 对客模型名 = 能力系列 + 输出档位：
//
//	video-enhance-{720p|1080p|2k|4k}   大模型增强（真人 / 漫剧 / 小脸优化 / AI 生成源，由 metadata.scene 选）
//	video-restore-{720p|1080p|2k|4k}   老片 / 低清修复（综合增强 + 人脸增强 + 去划痕）
//
// 上游按「输出分辨率 × 输出帧率档 × 输出时长」计费，其中分辨率由模板钉死、时长跟随源视频、
// 帧率默认随源。所以对客只按模型名（分辨率 + 系列）定单价，秒数与帧率档在任务完成后
// 用上游回报的实际输出校正。

const (
	ChannelName = "tencentmps"

	defaultEndpoint          = "https://mps.tencentcloudapi.com"
	apiVersion               = "2019-06-12"
	serviceName              = "mps"
	actionProcessMedia       = "ProcessMedia"
	actionDescribeTaskDetail = "DescribeTaskDetail"
)

const (
	familyEnhance = "video-enhance"
	familyRestore = "video-restore"
)

var ModelList = []string{
	"video-enhance-720p",
	"video-enhance-1080p",
	"video-enhance-2k",
	"video-enhance-4k",
	"video-restore-720p",
	"video-restore-1080p",
	"video-restore-2k",
	"video-restore-4k",
}

// resolutionShortEdge 是各输出档位的短边像素，必须与模板一致（上游要求 StdExtInfo 带 short）。
var resolutionShortEdge = map[string]int{
	"720p":  720,
	"1080p": 1080,
	"2k":    1440,
	"4k":    2160,
}

var resolutionOrder = []string{"720p", "1080p", "2k", "4k"}

// 场景决定用哪一套增强模板；同一档位下各场景单价相同，所以场景只是参数，不进模型名。
const (
	SceneReal           = "real"             // 真人实拍
	SceneAnime          = "anime"            // 漫剧 / 动画
	SceneRealSmallFace  = "real-small-face"  // 真人，远景小脸优化
	SceneAnimeSmallFace = "anime-small-face" // 漫剧，远景小脸优化
	SceneSeedance       = "seedance"         // AI 视频模型生成的源视频
)

var sceneOrder = []string{SceneReal, SceneAnime, SceneRealSmallFace, SceneAnimeSmallFace, SceneSeedance}

// enhanceTemplates 是上游预制的大模型增强模板 ID：scene → resolution → template。
var enhanceTemplates = map[string]map[string]int{
	SceneReal:           {"720p": 327001, "1080p": 327003, "2k": 327005, "4k": 327007},
	SceneAnime:          {"720p": 327002, "1080p": 327004, "2k": 327006, "4k": 327008},
	SceneRealSmallFace:  {"720p": 327025, "1080p": 327026, "2k": 327027, "4k": 327028},
	SceneAnimeSmallFace: {"720p": 327029, "1080p": 327030, "2k": 327031, "4k": 327032},
	SceneSeedance:       {"720p": 327033, "1080p": 327034, "2k": 327035, "4k": 327036},
}

// restoreTemplates 是上游预制的大模型修复模板 ID（老片 / 低清），按修复价计费。
var restoreTemplates = map[string]int{
	"720p": 327021, "1080p": 327022, "2k": 327023, "4k": 327024,
}

// 帧率档位：上游按输出帧率分三档计价，≤30 帧为基准价，≤60 帧两倍，≤120 帧四倍。
const (
	fpsTierBase   = 30
	fpsTierDouble = 60
)
