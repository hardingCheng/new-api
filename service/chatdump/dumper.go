package chatdump

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// chatdump 抓取经过网关的请求/响应。通过环境变量配置：
//   CHAT_DUMP_ENABLED=false      关闭（默认开启）
//   CHAT_DUMP_DIR=./chatdump     落盘目录
//   CHAT_DUMP_MODEL_KEYS=opus    模型关键字（逗号分隔，子串匹配）；设为 * 则全量抓取所有模型
//   CHAT_DUMP_MIN_FREE_MB=2048   磁盘剩余低于此值(MB)时暂停写入，防止写满磁盘导致宕机；0 关闭保护
var (
	dumpDir          string
	dumpEnabled      bool
	dumpAll          bool   // CHAT_DUMP_MODEL_KEYS=* 时为 true，抓取所有非空模型
	dumpModelKeys    []string
	dumpMinFreeBytes uint64 // 磁盘剩余低于此值则暂停写入（硬保护）
	once             sync.Once
	writeMu          sync.Mutex
	lastDiskWarn     time.Time
)

func initConfig() {
	once.Do(func() {
		dumpEnabled = common.GetEnvOrDefaultBool("CHAT_DUMP_ENABLED", true)
		dumpDir = common.GetEnvOrDefaultString("CHAT_DUMP_DIR", "./chatdump")
		// 默认只抓 opus；可用逗号分隔自定义关键字；设为 * 则全量抓取
		keys := common.GetEnvOrDefaultString("CHAT_DUMP_MODEL_KEYS", "opus")
		for _, k := range strings.Split(keys, ",") {
			k = strings.TrimSpace(strings.ToLower(k))
			if k == "*" {
				dumpAll = true
			} else if k != "" {
				dumpModelKeys = append(dumpModelKeys, k)
			}
		}
		// 磁盘保护：剩余低于阈值则暂停写入，防止写满磁盘宕机（默认 2GB，0 关闭）
		dumpMinFreeBytes = uint64(common.GetEnvOrDefault("CHAT_DUMP_MIN_FREE_MB", 2048)) * 1024 * 1024
		if dumpEnabled {
			if err := os.MkdirAll(dumpDir, 0o755); err != nil {
				common.SysLog(fmt.Sprintf("chatdump: 创建目录 %s 失败: %s", dumpDir, err.Error()))
				dumpEnabled = false
			} else {
				scope := fmt.Sprintf("关键字=%v", dumpModelKeys)
				if dumpAll {
					scope = "全量(所有模型)"
				}
				common.SysLog(fmt.Sprintf("chatdump: 已启用，目录=%s %s 磁盘保护阈值=%dMB", dumpDir, scope, dumpMinFreeBytes/1024/1024))
			}
		}
	})
}

// clientHeaderWhitelist 用于识别 harness/SDK 的请求头(全部非密钥);其余(尤其 Authorization/x-api-key)一律不记。
var clientHeaderWhitelist = []string{
	"X-Stainless-Lang", "X-Stainless-Package-Version", "X-Stainless-Runtime",
	"X-Stainless-Runtime-Version", "X-Stainless-Os", "X-Stainless-Arch", "X-Stainless-Retry-Count",
	"Anthropic-Version", "Anthropic-Beta", "Anthropic-Dangerous-Direct-Browser-Access",
	"X-App", "X-Title", "Http-Referer", "X-Client-Name", "X-Client-Version",
}

// ExtractClientHeaders 从 gin 请求头按白名单提取识别用 header(供 relay handler 调用)。
func ExtractClientHeaders(get func(string) string) map[string]string {
	out := map[string]string{}
	for _, h := range clientHeaderWhitelist {
		if v := get(h); v != "" {
			out[h] = v
		}
	}
	return out
}

// ShouldDump 判定模型是否需要抓取。空字符串模型不抓。
func ShouldDump(model string) bool {
	initConfig()
	if !dumpEnabled || model == "" {
		return false
	}
	if dumpAll {
		return true
	}
	lower := strings.ToLower(model)
	for _, k := range dumpModelKeys {
		if strings.Contains(lower, k) {
			return true
		}
	}
	return false
}

// Record 单次对话完整记录。
type Record struct {
	Timestamp     string          `json:"timestamp"`
	DumpID        string          `json:"dump_id"`
	UpstreamModel string          `json:"upstream_model"`
	OriginModel   string          `json:"origin_model"`
	UserID        int             `json:"user_id"`
	UserEmail     string          `json:"user_email,omitempty"`
	TokenID       int             `json:"token_id"`
	ChannelID     int             `json:"channel_id"`
	ChannelType   int             `json:"channel_type"`
	RelayFormat   string          `json:"relay_format"`
	Stream        bool            `json:"stream"`
	IP            string          `json:"ip,omitempty"`
	UserAgent     string          `json:"user_agent,omitempty"`
	RequestPath   string          `json:"request_path,omitempty"`
	// 客户端识别 header 白名单(SDK语言/版本、anthropic-beta、聚合器标识等),用于识别 harness;不含密钥
	ClientHeaders map[string]string `json:"client_headers,omitempty"`
	DurationMs    int64           `json:"duration_ms,omitempty"`

	// 完整入参（系统提示词、消息、tools、thinking 配置等都在里面）
	Request json.RawMessage `json:"request,omitempty"`

	// 非流式：完整响应 JSON。
	Response json.RawMessage `json:"response,omitempty"`

	// 流式：原始 SSE 事件按顺序排列（保留所有思考链 thinking_delta / signature_delta 等）
	StreamEvents []StreamEvent `json:"stream_events,omitempty"`

	// 流式聚合后的便捷字段
	AggregatedText     string `json:"aggregated_text,omitempty"`
	AggregatedThinking string `json:"aggregated_thinking,omitempty"`

	// usage 摘要（来源于响应自身或聚合）
	Usage json.RawMessage `json:"usage,omitempty"`

	Error string `json:"error,omitempty"`
}

// StreamEvent 一个 SSE 事件。data 保留原始 JSON。
type StreamEvent struct {
	Index int             `json:"i"`
	At    int64           `json:"t_ms"` // 距离开始的毫秒
	Type  string          `json:"type,omitempty"`
	Data  json.RawMessage `json:"data"`
}

// Session 单次请求的运行期累积器。
type Session struct {
	mu        sync.Mutex
	startTime time.Time
	rec       Record
	enabled   bool

	textBuilder     strings.Builder
	thinkingBuilder strings.Builder
}

// NewSession 创建一次抓取会话。model 用于决定是否启用。
func NewSession(model string) *Session {
	initConfig()
	s := &Session{
		startTime: time.Now(),
		enabled:   ShouldDump(model),
	}
	if s.enabled {
		s.rec.DumpID = common.GetUUID()
		s.rec.Timestamp = s.startTime.Format("2006-01-02T15:04:05.000Z07:00")
		s.rec.UpstreamModel = model
		s.rec.OriginModel = model
	}
	return s
}

func (s *Session) Enabled() bool {
	if s == nil {
		return false
	}
	return s.enabled
}

// SetMeta 写入请求级元数据。
func (s *Session) SetMeta(meta map[string]any) {
	if !s.Enabled() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := meta["upstream_model"].(string); ok && v != "" {
		s.rec.UpstreamModel = v
	}
	if v, ok := meta["origin_model"].(string); ok && v != "" {
		s.rec.OriginModel = v
	}
	if v, ok := meta["user_id"].(int); ok {
		s.rec.UserID = v
	}
	if v, ok := meta["user_email"].(string); ok {
		s.rec.UserEmail = v
	}
	if v, ok := meta["token_id"].(int); ok {
		s.rec.TokenID = v
	}
	if v, ok := meta["channel_id"].(int); ok {
		s.rec.ChannelID = v
	}
	if v, ok := meta["channel_type"].(int); ok {
		s.rec.ChannelType = v
	}
	if v, ok := meta["relay_format"].(string); ok {
		s.rec.RelayFormat = v
	}
	if v, ok := meta["stream"].(bool); ok {
		s.rec.Stream = v
	}
	if v, ok := meta["ip"].(string); ok {
		s.rec.IP = v
	}
	if v, ok := meta["user_agent"].(string); ok {
		s.rec.UserAgent = v
	}
	if v, ok := meta["request_path"].(string); ok {
		s.rec.RequestPath = v
	}
	if v, ok := meta["client_headers"].(map[string]string); ok && len(v) > 0 {
		s.rec.ClientHeaders = v
	}
}

// SetRequest 保存原始请求体。会复制一份，调用方传入的 slice 可以复用。
func (s *Session) SetRequest(body []byte) {
	if !s.Enabled() || len(body) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(body))
	copy(cp, body)
	if json.Valid(cp) {
		s.rec.Request = cp
	} else {
		// 不是 JSON 就 base64 包一层
		quoted, _ := json.Marshal(string(cp))
		s.rec.Request = quoted
	}
}

// SetResponse 保存非流式响应体。
func (s *Session) SetResponse(body []byte) {
	if !s.Enabled() || len(body) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(body))
	copy(cp, body)
	if json.Valid(cp) {
		s.rec.Response = cp
	} else {
		quoted, _ := json.Marshal(string(cp))
		s.rec.Response = quoted
	}
}

// AppendStreamEvent 追加一条 SSE event 的 data。data 必须是合法 JSON。
func (s *Session) AppendStreamEvent(eventType string, data []byte) {
	if !s.Enabled() || len(data) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	raw := json.RawMessage(cp)
	if !json.Valid(cp) {
		quoted, _ := json.Marshal(string(cp))
		raw = quoted
	}
	s.rec.StreamEvents = append(s.rec.StreamEvents, StreamEvent{
		Index: len(s.rec.StreamEvents),
		At:    time.Since(s.startTime).Milliseconds(),
		Type:  eventType,
		Data:  raw,
	})
}

// AppendText 累积模型回复文本（流式）。
func (s *Session) AppendText(t string) {
	if !s.Enabled() || t == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.textBuilder.WriteString(t)
}

// AppendThinking 累积思考内容（流式）。
func (s *Session) AppendThinking(t string) {
	if !s.Enabled() || t == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.thinkingBuilder.WriteString(t)
}

// SetUsage 保存 usage 信息（任何可序列化对象）。
func (s *Session) SetUsage(u any) {
	if !s.Enabled() || u == nil {
		return
	}
	b, err := json.Marshal(u)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec.Usage = b
}

// SetError 标记错误信息。
func (s *Session) SetError(msg string) {
	if !s.Enabled() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec.Error = msg
}

// Flush 写盘。即便没有响应也会写出，便于排查超时/错误。
func (s *Session) Flush() {
	if !s.Enabled() {
		return
	}
	s.mu.Lock()
	s.rec.DurationMs = time.Since(s.startTime).Milliseconds()
	s.rec.AggregatedText = s.textBuilder.String()
	s.rec.AggregatedThinking = s.thinkingBuilder.String()
	rec := s.rec
	s.mu.Unlock()

	go writeRecord(&rec)
}

// hasEnoughDiskSpace 检查 dumpDir 所在分区剩余空间是否达到阈值。
// 全量抓取下这是防止写满磁盘导致全站宕机的硬保护；调用方已持 writeMu。
func hasEnoughDiskSpace() bool {
	if dumpMinFreeBytes == 0 {
		return true
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(dumpDir, &st); err != nil {
		return true // 检查失败不阻断正常写入
	}
	avail := st.Bavail * uint64(st.Bsize)
	if avail < dumpMinFreeBytes {
		if time.Since(lastDiskWarn) > time.Minute { // 限流,避免刷屏
			lastDiskWarn = time.Now()
			common.SysLog(fmt.Sprintf("chatdump: 磁盘剩余 %dMB < 阈值 %dMB，暂停写入以保护磁盘",
				avail/1024/1024, dumpMinFreeBytes/1024/1024))
		}
		return false
	}
	return true
}

func writeRecord(rec *Record) {
	defer func() {
		if r := recover(); r != nil {
			common.SysLog(fmt.Sprintf("chatdump: 写盘 panic: %v", r))
		}
	}()

	day := time.Now().Format("2006-01-02")
	dir := filepath.Join(dumpDir, day)

	writeMu.Lock()
	defer writeMu.Unlock()

	// 磁盘保护：剩余不足时丢弃本条，绝不写满磁盘
	if !hasEnoughDiskSpace() {
		return
	}

	// 脱敏：删邮箱/IP(PII)；保留 user_agent + client_headers 用于识别 harness；模型数据全保留
	rec.UserEmail = ""
	rec.IP = ""

	if err := os.MkdirAll(dir, 0o755); err != nil {
		common.SysLog(fmt.Sprintf("chatdump: 创建子目录失败 %s: %s", dir, err.Error()))
		return
	}

	safeModel := sanitizeFileName(rec.UpstreamModel)
	stamp := time.Now().Format("150405.000")
	stamp = strings.ReplaceAll(stamp, ".", "")
	name := fmt.Sprintf("%s_%s_u%d_%s.json", stamp, safeModel, rec.UserID, rec.DumpID[:8])
	path := filepath.Join(dir, name)

	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		common.SysLog(fmt.Sprintf("chatdump: 序列化失败: %s", err.Error()))
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		common.SysLog(fmt.Sprintf("chatdump: 写入 %s 失败: %s", path, err.Error()))
		return
	}
}

func sanitizeFileName(s string) string {
	if s == "" {
		return "unknown"
	}
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	out := r.Replace(s)
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}
