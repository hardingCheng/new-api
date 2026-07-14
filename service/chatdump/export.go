package chatdump

// 把抓到的原始 dump（Anthropic 原生格式）批量加工为符合验收标准的数据集：
//   - 转换为 OpenAI 标准 trajectory（含 reasoning_content / tool_calls / tool 消息）
//   - 同 user_id 内做对话前缀聚合（短前缀被更长的对话淘汰）
//   - 按 thinking 等级、reasoning 占比、总轮次等条件筛选
//   - 关键词法做 13 类领域分类 + scaffold 识别（基于 user_agent / system 提示）
//
// 入口函数：BuildDataset。

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ExportOptions 控制筛选阈值。
type ExportOptions struct {
	MinTurns               int     // 总轮次阈值，> 此值才保留；默认 5
	MinThinkingBudget      int     // thinking budget 阈值（high≈4096）；默认 4096
	MinReasoningRatio      float64 // assistant turn 中 reasoning 非空占比；默认 0.5
	RequireThinking        bool    // 是否要求请求中开了 thinking；默认 true
	IncludeUnknownDomain   bool    // 没分类到的是否保留；默认 true
	DateFilter             string  // YYYY-MM-DD，留空=全部
	ModelFilter            string  // 模型关键字
}

// DefaultExportOptions 一组保守默认值，对应"采购验收标准"。
func DefaultExportOptions() ExportOptions {
	return ExportOptions{
		MinTurns:             5,
		MinThinkingBudget:    4096, // ≥ high
		MinReasoningRatio:    0.5,
		RequireThinking:      true,
		IncludeUnknownDomain: true,
	}
}

// ============================================================
// OpenAI 标准 trajectory 数据结构
// ============================================================

type Trajectory struct {
	ModelConfig ModelConfig    `json:"model_config"`
	Tools       []OAITool      `json:"tools,omitempty"`
	Messages    []OAIMessage   `json:"messages"`
	Metadata    ExportMetadata `json:"metadata,omitempty"`
}

type ModelConfig struct {
	Model       string   `json:"model"`
	Provider    string   `json:"provider"`
	Temperature *float64 `json:"temperature,omitempty"`
	Thinking    string   `json:"thinking,omitempty"` // off/low/medium/high/max
	MaxTokens   int      `json:"max_tokens,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	TopK        *int     `json:"top_k,omitempty"`
}

type OAITool struct {
	Type     string          `json:"type"`
	Function OAIToolFunction `json:"function"`
}

type OAIToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type OAIMessage struct {
	Role             string        `json:"role"`
	Content          any           `json:"content"`
	ReasoningContent string        `json:"reasoning_content,omitempty"`
	Name             string        `json:"name,omitempty"`
	ToolCalls        []OAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string        `json:"tool_call_id,omitempty"`
}

type OAIToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function OAIFunctionCall `json:"function"`
}

type OAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ExportMetadata struct {
	Domain         string  `json:"domain,omitempty"`
	Scaffold       string  `json:"scaffold,omitempty"`
	UserID         int     `json:"user_id,omitempty"`
	Turns          int     `json:"turns,omitempty"`
	ReasoningRatio float64 `json:"reasoning_ratio,omitempty"`
	SourceDumps    []string `json:"source_dumps,omitempty"`
}

// ============================================================
// Anthropic 原生入参/响应解析结构（够用即可，不引入 dto 包避免循环依赖）
// ============================================================

type rawRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature *float64        `json:"temperature"`
	TopP        *float64        `json:"top_p"`
	TopK        *int            `json:"top_k"`
	System      json.RawMessage `json:"system"`
	Messages    []rawMessage    `json:"messages"`
	Tools       []rawTool       `json:"tools"`
	Thinking    *rawThinking    `json:"thinking"`
}

type rawThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

type rawTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type rawMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// content 块通用形态
type rawContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	Signature string          `json:"signature,omitempty"`

	// tool_use
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`

	// tool_result
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`

	// image
	Source *struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	} `json:"source,omitempty"`
}

type rawResponse struct {
	ID         string            `json:"id"`
	Model      string            `json:"model"`
	Content    []rawContentBlock `json:"content"`
	StopReason string            `json:"stop_reason"`
}

// ============================================================
// Pipeline
// ============================================================

// BuildDataset 读取原始 dump 文件并产出验收数据集。
func BuildDataset(opts ExportOptions) ([]*Trajectory, ExportStats, error) {
	stats := ExportStats{}
	files, err := ListFiles(ListFilter{Date: opts.DateFilter, Model: opts.ModelFilter})
	if err != nil {
		return nil, stats, err
	}
	stats.SourceCount = len(files)

	// Step 1: 加载并转换为 trajectory
	var trajs []*Trajectory
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(DumpRoot(), f.Date, f.Name))
		if err != nil {
			stats.Errors++
			continue
		}
		var rec Record
		if err := json.Unmarshal(raw, &rec); err != nil {
			stats.Errors++
			continue
		}
		t, err := convertRecord(&rec, f.Name)
		if err != nil {
			stats.Errors++
			continue
		}
		trajs = append(trajs, t)
	}
	stats.ConvertedCount = len(trajs)

	// Step 2: 前缀聚合
	trajs = dedupeByPrefix(trajs)
	stats.AfterDedupe = len(trajs)

	// Step 3: 筛选
	out := make([]*Trajectory, 0, len(trajs))
	for _, t := range trajs {
		if !passFilters(t, opts) {
			continue
		}
		out = append(out, t)
	}
	stats.AfterFilter = len(out)

	// Step 4: 打标签
	for _, t := range out {
		t.Metadata.Domain = classifyDomain(t)
		t.Metadata.Scaffold = detectScaffold(t)
	}

	// Step 5: 排序，便于查看
	sort.Slice(out, func(i, j int) bool {
		return out[i].Metadata.Turns > out[j].Metadata.Turns
	})

	return out, stats, nil
}

type ExportStats struct {
	SourceCount    int `json:"source_count"`
	ConvertedCount int `json:"converted_count"`
	AfterDedupe    int `json:"after_dedupe"`
	AfterFilter    int `json:"after_filter"`
	Errors         int `json:"errors"`
}

// ============================================================
// Step 1: 单条 dump → trajectory
// ============================================================

func convertRecord(rec *Record, fileName string) (*Trajectory, error) {
	if len(rec.Request) == 0 {
		return nil, fmt.Errorf("empty request")
	}
	var req rawRequest
	if err := json.Unmarshal(rec.Request, &req); err != nil {
		return nil, err
	}

	cfg := ModelConfig{
		Model:       req.Model,
		Provider:    "anthropic",
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		TopK:        req.TopK,
	}
	if req.Thinking != nil {
		cfg.Thinking = thinkingLevel(req.Thinking.BudgetTokens)
	}

	// tools
	var tools []OAITool
	for _, t := range req.Tools {
		tools = append(tools, OAITool{
			Type: "function",
			Function: OAIToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	// messages: system → user/assistant 历史 → 本次 assistant 响应
	msgs := make([]OAIMessage, 0)
	if sys := parseSystemText(req.System); sys != "" {
		msgs = append(msgs, OAIMessage{Role: "system", Content: sys})
	}
	for _, m := range req.Messages {
		converted := convertHistMessage(m)
		msgs = append(msgs, converted...)
	}
	if asst := buildAssistantFromResponse(rec); asst != nil {
		msgs = append(msgs, *asst)
	}

	return &Trajectory{
		ModelConfig: cfg,
		Tools:       tools,
		Messages:    msgs,
		Metadata: ExportMetadata{
			UserID:      rec.UserID,
			Turns:       len(msgs),
			SourceDumps: []string{fileName},
		},
	}, nil
}

func parseSystemText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// string?
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// array of {type, text}
	var arr []rawContentBlock
	if err := json.Unmarshal(raw, &arr); err == nil {
		var sb strings.Builder
		for _, b := range arr {
			if b.Type == "text" || b.Type == "" {
				if b.Text != "" {
					if sb.Len() > 0 {
						sb.WriteString("\n")
					}
					sb.WriteString(b.Text)
				}
			}
		}
		return sb.String()
	}
	return ""
}

// 转换一条历史 message。可能拆成多条（assistant 含 tool_use → 1 条 assistant + 同时把 tool_result 拆出去）。
func convertHistMessage(m rawMessage) []OAIMessage {
	// content 可能是 string 或 []rawContentBlock
	if m.Role == "user" {
		return convertUserMessage(m)
	}
	if m.Role == "assistant" {
		return []OAIMessage{convertAssistantMessage(m)}
	}
	// system 之类按文本处理
	return []OAIMessage{{Role: m.Role, Content: parseStringOrText(m.Content)}}
}

func convertUserMessage(m rawMessage) []OAIMessage {
	// 纯字符串
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		return []OAIMessage{{Role: "user", Content: s}}
	}
	var blocks []rawContentBlock
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return []OAIMessage{{Role: "user", Content: string(m.Content)}}
	}
	// user 块里可能混 text、image、tool_result
	var (
		userParts []map[string]any
		userText  strings.Builder
		toolMsgs  []OAIMessage
		hasMedia  bool
	)
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if userText.Len() > 0 {
				userText.WriteString("\n")
			}
			userText.WriteString(b.Text)
			userParts = append(userParts, map[string]any{"type": "text", "text": b.Text})
		case "image":
			hasMedia = true
			if b.Source != nil {
				url := "data:" + b.Source.MediaType + ";base64," + b.Source.Data
				userParts = append(userParts, map[string]any{
					"type":      "image_url",
					"image_url": map[string]any{"url": url},
				})
			}
		case "tool_result":
			content := stringifyToolResultContent(b.Content)
			toolMsgs = append(toolMsgs, OAIMessage{
				Role:       "tool",
				ToolCallID: b.ToolUseID,
				Content:    content,
			})
		}
	}
	out := make([]OAIMessage, 0, 1+len(toolMsgs))
	if hasMedia {
		// 多模态格式
		if len(userParts) > 0 {
			out = append(out, OAIMessage{Role: "user", Content: userParts})
		}
	} else if userText.Len() > 0 {
		out = append(out, OAIMessage{Role: "user", Content: userText.String()})
	}
	out = append(out, toolMsgs...)
	return out
}

func convertAssistantMessage(m rawMessage) OAIMessage {
	out := OAIMessage{Role: "assistant"}
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		out.Content = s
		return out
	}
	var blocks []rawContentBlock
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		out.Content = string(m.Content)
		return out
	}
	var (
		text     strings.Builder
		thinking strings.Builder
		calls    []OAIToolCall
	)
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if text.Len() > 0 {
				text.WriteString("\n")
			}
			text.WriteString(b.Text)
		case "thinking":
			if thinking.Len() > 0 {
				thinking.WriteString("\n")
			}
			thinking.WriteString(b.Thinking)
		case "tool_use":
			args, _ := json.Marshal(b.Input)
			calls = append(calls, OAIToolCall{
				ID:   b.ID,
				Type: "function",
				Function: OAIFunctionCall{
					Name:      b.Name,
					Arguments: string(args),
				},
			})
		}
	}
	if text.Len() > 0 {
		out.Content = text.String()
	} else {
		out.Content = nil
	}
	if thinking.Len() > 0 {
		out.ReasoningContent = thinking.String()
	}
	if len(calls) > 0 {
		out.ToolCalls = calls
	}
	return out
}

func parseStringOrText(raw json.RawMessage) any {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

func stringifyToolResultContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []rawContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var sb strings.Builder
		for _, b := range blocks {
			if b.Text != "" {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(b.Text)
			}
		}
		return sb.String()
	}
	return string(raw)
}

// 从 dump 中重建本次 assistant 响应。流式：聚合的 text 和 thinking 就是它；非流式：解析 response.content。
// 还会重建 tool_calls。
func buildAssistantFromResponse(rec *Record) *OAIMessage {
	out := &OAIMessage{Role: "assistant"}

	if len(rec.Response) > 0 {
		// 非流式响应：直接解析 content 块
		var resp rawResponse
		if err := json.Unmarshal(rec.Response, &resp); err == nil && len(resp.Content) > 0 {
			var (
				text     strings.Builder
				thinking strings.Builder
				calls    []OAIToolCall
			)
			for _, b := range resp.Content {
				switch b.Type {
				case "text":
					if text.Len() > 0 {
						text.WriteString("\n")
					}
					text.WriteString(b.Text)
				case "thinking":
					if thinking.Len() > 0 {
						thinking.WriteString("\n")
					}
					thinking.WriteString(b.Thinking)
				case "tool_use":
					args, _ := json.Marshal(b.Input)
					calls = append(calls, OAIToolCall{
						ID: b.ID, Type: "function",
						Function: OAIFunctionCall{Name: b.Name, Arguments: string(args)},
					})
				}
			}
			if text.Len() > 0 {
				out.Content = text.String()
			}
			if thinking.Len() > 0 {
				out.ReasoningContent = thinking.String()
			}
			if len(calls) > 0 {
				out.ToolCalls = calls
			}
			if out.Content == nil && out.ReasoningContent == "" && len(out.ToolCalls) == 0 {
				return nil
			}
			return out
		}
	}

	// 流式：从 stream_events 中聚合（dumper 已经把 aggregated_text/thinking 算好，但 tool_calls 需要重建）
	out.Content = rec.AggregatedText
	if out.Content == "" {
		out.Content = nil
	}
	if rec.AggregatedThinking != "" {
		out.ReasoningContent = rec.AggregatedThinking
	}
	if calls := rebuildToolCallsFromStream(rec.StreamEvents); len(calls) > 0 {
		out.ToolCalls = calls
	}
	if out.Content == nil && out.ReasoningContent == "" && len(out.ToolCalls) == 0 {
		return nil
	}
	return out
}

// 从流式事件里把 tool_use 的分片 input_json_delta 拼回完整 arguments JSON。
func rebuildToolCallsFromStream(events []StreamEvent) []OAIToolCall {
	type tcAcc struct {
		id   string
		name string
		args strings.Builder
	}
	idxToTc := map[int]*tcAcc{}
	for _, e := range events {
		var ev struct {
			Type         string          `json:"type"`
			Index        int             `json:"index"`
			ContentBlock *rawContentBlock `json:"content_block"`
			Delta        *rawContentBlock `json:"delta"`
		}
		// delta 用 rawContentBlock 装会丢 partial_json，这里另开一段
		if err := json.Unmarshal(e.Data, &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "content_block_start":
			if ev.ContentBlock != nil && ev.ContentBlock.Type == "tool_use" {
				idxToTc[ev.Index] = &tcAcc{id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
			}
		case "content_block_delta":
			// 二次解析拿 partial_json
			var d struct {
				Delta struct {
					Type        string `json:"type"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
				Index int `json:"index"`
			}
			_ = json.Unmarshal(e.Data, &d)
			if d.Delta.Type == "input_json_delta" {
				if acc, ok := idxToTc[d.Index]; ok {
					acc.args.WriteString(d.Delta.PartialJSON)
				}
			}
		}
	}
	if len(idxToTc) == 0 {
		return nil
	}
	idxs := make([]int, 0, len(idxToTc))
	for i := range idxToTc {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	out := make([]OAIToolCall, 0, len(idxs))
	for _, i := range idxs {
		acc := idxToTc[i]
		args := acc.args.String()
		if args == "" {
			args = "{}"
		}
		out = append(out, OAIToolCall{
			ID: acc.id, Type: "function",
			Function: OAIFunctionCall{Name: acc.name, Arguments: args},
		})
	}
	return out
}

// thinking budget → 等级
func thinkingLevel(budget int) string {
	switch {
	case budget <= 0:
		return "off"
	case budget < 1024:
		return "off"
	case budget < 2048:
		return "low"
	case budget < 4096:
		return "medium"
	case budget < 8192:
		return "high"
	default:
		return "max"
	}
}

// ============================================================
// Step 2: session 聚合（同 user_id 内前缀匹配）
// ============================================================

func dedupeByPrefix(trajs []*Trajectory) []*Trajectory {
	groups := map[int][]*Trajectory{}
	for _, t := range trajs {
		groups[t.Metadata.UserID] = append(groups[t.Metadata.UserID], t)
	}
	var out []*Trajectory
	for _, group := range groups {
		// 按消息数升序：短的在前
		sort.Slice(group, func(i, j int) bool {
			return len(group[i].Messages) < len(group[j].Messages)
		})
		// 把每条 traj 的"前缀指纹"算出来
		fps := make([][]string, len(group))
		for i, t := range group {
			fps[i] = messageFingerprints(t.Messages)
		}
		dropped := make([]bool, len(group))
		for i := 0; i < len(group); i++ {
			if dropped[i] {
				continue
			}
			for j := i + 1; j < len(group); j++ {
				if dropped[j] {
					continue
				}
				if isPrefixOf(fps[i], fps[j]) {
					// j 包含 i：把 i 的 source 合并进 j 后丢弃 i
					group[j].Metadata.SourceDumps = append(group[j].Metadata.SourceDumps, group[i].Metadata.SourceDumps...)
					dropped[i] = true
					break
				}
			}
		}
		for i, t := range group {
			if !dropped[i] {
				t.Metadata.Turns = len(t.Messages)
				out = append(out, t)
			}
		}
	}
	return out
}

func messageFingerprints(msgs []OAIMessage) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		h := sha1.New()
		h.Write([]byte(m.Role))
		h.Write([]byte{'|'})
		h.Write([]byte(stringifyContent(m.Content)))
		if len(m.ToolCalls) > 0 {
			b, _ := json.Marshal(m.ToolCalls)
			h.Write(b)
		}
		if m.ToolCallID != "" {
			h.Write([]byte(m.ToolCallID))
		}
		out[i] = hex.EncodeToString(h.Sum(nil))
	}
	return out
}

func stringifyContent(c any) string {
	if c == nil {
		return ""
	}
	if s, ok := c.(string); ok {
		return s
	}
	b, _ := json.Marshal(c)
	return string(b)
}

func isPrefixOf(a, b []string) bool {
	if len(a) == 0 || len(a) >= len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ============================================================
// Step 3: 筛选
// ============================================================

func passFilters(t *Trajectory, opts ExportOptions) bool {
	if opts.MinTurns > 0 && t.Metadata.Turns <= opts.MinTurns {
		return false
	}
	if opts.RequireThinking {
		switch t.ModelConfig.Thinking {
		case "off", "":
			return false
		}
	}
	// thinking 等级阈值
	if opts.MinThinkingBudget >= 4096 && t.ModelConfig.Thinking != "high" && t.ModelConfig.Thinking != "max" {
		return false
	} else if opts.MinThinkingBudget >= 2048 && t.ModelConfig.Thinking == "low" {
		return false
	}
	// reasoning 占比
	asstTotal, asstWithReason := 0, 0
	for _, m := range t.Messages {
		if m.Role != "assistant" {
			continue
		}
		asstTotal++
		if strings.TrimSpace(m.ReasoningContent) != "" {
			asstWithReason++
		}
	}
	if asstTotal == 0 {
		return false
	}
	ratio := float64(asstWithReason) / float64(asstTotal)
	t.Metadata.ReasoningRatio = ratio
	if ratio < opts.MinReasoningRatio {
		return false
	}
	return true
}

// ============================================================
// Step 4: 关键词分类 + scaffold 识别
// ============================================================

// 13 类关键词。中英文混合，全部小写匹配。
// 注：每类挑了高区分度词；规则法不求精确，只求大致分桶。
var domainKeywords = map[string][]string{
	"development": {
		"代码", "函数", "重构", "bug", "debug", "调试", "git ", "commit", "pull request", "merge",
		"编译", "build", "单元测试", "unit test", "interface", "接口", "异常", "exception",
		"package", "import ", "class ", "func ", "def ", "返回值", "变量", "依赖",
		"前端", "后端", "全栈", "react", "vue", "next.js", "django", "flask", "spring",
	},
	"system_admin": {
		"systemctl", "nginx", "apache", "ssh ", "scp", "rsync", "chmod", "chown", "umask",
		"防火墙", "iptables", "ufw", "selinux", "crontab", "yum ", "apt ", "apt-get",
		"docker run", "docker exec", "kubectl", "k8s", "/etc/", "/var/log",
	},
	"data_analysis": {
		"分析", "统计", "聚合", "透视", "csv", "excel", "spreadsheet", "可视化",
		"清洗", "去重", "归一化", "groupby", "join 表", "数据透视", "outlier",
		"pandas", "polars", "describe(", "dataframe", "jupyter", "notebook",
	},
	"research": {
		"搜索", "调研", "对比", "评测", "选型", "百科", "wikipedia", "查一下",
		"论文", "综述", "literature review", "cite", "原理", "概念",
	},
	"content_creation": {
		"写一篇", "撰写", "翻译成", "润色", "扩写", "改写", "摘要", "summary", "总结一下",
		"博客", "公众号", "推文", "标题", "slogan", "文案", "剧本", "故事", "小说",
		"营销", "邀请函", "演讲稿",
	},
	"communication": {
		"slack", "discord", "telegram", "wechat", "邮件", "email", "回复客户", "回邮件",
		"群聊", "通知", "告警", "推送消息", "im 集成",
	},
	"media_processing": {
		"ocr", "图片识别", "图像生成", "图生图", "文生图", "midjourney", "stable diffusion",
		"截图", "pdf 提取", "pdf 转", "音频", "asr", "tts", "语音转文字", "字幕",
		"视频剪辑", "封面", "缩略图", "ffmpeg",
	},
	"automation": {
		"自动化", "工作流", "workflow", "n8n", "zapier", "make.com", "robotic",
		"定时任务自动", "agent ", "智能体", "编排", "orchestrat", "browser-use",
	},
	"monitoring": {
		"监控", "告警", "健康检查", "heartbeat", "uptime", "prometheus", "grafana",
		"sla", "运行时长", "心跳", "巡检", "告警阈值",
	},
	"scheduling": {
		"日程", "日历", "calendar", "提醒我", "todo", "待办", "排课", "排班",
		"meeting", "约会议", "今天几号", "下周",
	},
	"knowledge_mgmt": {
		"知识库", "笔记", "obsidian", "notion", "wiki", "rag", "向量库",
		"embedding 库", "记忆", "memory bank", "agent 配置",
	},
	"finance": {
		"股票", "k 线", "技术指标", "macd", "rsi", "回测", "backtest", "策略",
		"持仓", "止盈止损", "期货", "期权", "基金", "etf", "黄金价格", "汇率", "btc", "比特币",
	},
	"crm": {
		"客户", "线索", "lead", "成单", "crm", "电商", "店铺", "运营", "投放", "广告 roi",
		"私域", "复购", "客服话术", "工单",
	},
}

func classifyDomain(t *Trajectory) string {
	text := strings.ToLower(extractDomainText(t))
	if text == "" {
		return ""
	}
	bestDomain := ""
	bestScore := 0
	for domain, kws := range domainKeywords {
		score := 0
		for _, kw := range kws {
			if strings.Contains(text, strings.ToLower(kw)) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestDomain = domain
		}
	}
	return bestDomain
}

func extractDomainText(t *Trajectory) string {
	var sb strings.Builder
	limit := 4000
	for _, m := range t.Messages {
		if m.Role == "system" || m.Role == "user" {
			s := stringifyContent(m.Content)
			if s != "" {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(s)
				if sb.Len() > limit {
					break
				}
			}
		}
	}
	if sb.Len() > limit {
		return sb.String()[:limit]
	}
	return sb.String()
}

// scaffold 识别：基于第一条来源 dump 的 user_agent + system 文本
func detectScaffold(t *Trajectory) string {
	sys := ""
	for _, m := range t.Messages {
		if m.Role == "system" {
			sys = strings.ToLower(stringifyContent(m.Content))
			break
		}
	}
	// user_agent 在 trajectory 里没存——单独从源 dump 第一个文件再读
	ua := ""
	if len(t.Metadata.SourceDumps) > 0 {
		// 尝试根据文件名匹配；read 失败就跳过
		ua = lookupUserAgent(t.Metadata.SourceDumps[0])
		ua = strings.ToLower(ua)
	}

	switch {
	case strings.Contains(ua, "claude-cli") || strings.Contains(sys, "you are claude code"):
		return "claude_code"
	case strings.Contains(ua, "anthropic-sdk") || strings.Contains(ua, "anthropic-python") || strings.Contains(ua, "anthropic-typescript") || strings.Contains(ua, "@anthropic-ai/sdk"):
		return "anthropic_sdk"
	case strings.Contains(ua, "openclaw") || strings.Contains(sys, "openclaw"):
		return "openclaw"
	case strings.Contains(ua, "hermes"):
		return "hermes"
	case strings.Contains(ua, "cherry"):
		return "cherry_studio"
	case strings.Contains(ua, "openrouter"):
		return "openrouter"
	case strings.Contains(ua, "cursor"):
		return "cursor"
	case strings.Contains(ua, "cline"):
		return "cline"
	case strings.Contains(ua, "continue"):
		return "continue"
	case strings.Contains(ua, "openai") && strings.Contains(ua, "python"):
		return "openai_sdk"
	case strings.Contains(ua, "curl"):
		return "raw_curl"
	}
	return "unknown"
}

// 简单根据 dump 文件名读 user_agent（dump 文件已存在）
func lookupUserAgent(fileName string) string {
	// 文件名前缀的 date 信息不在传入参数里——这里做兜底：在 dump 根目录下扫一遍。
	// 真正读起来 IO 很轻，每个 trajectory 调一次。
	dates, _ := ListDates()
	for _, d := range dates {
		path := filepath.Join(DumpRoot(), d, fileName)
		if data, err := os.ReadFile(path); err == nil {
			var rec Record
			if json.Unmarshal(data, &rec) == nil {
				return rec.UserAgent
			}
		}
	}
	return ""
}
