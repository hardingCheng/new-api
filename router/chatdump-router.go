package router

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/chatdump"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// SetChatDumpRouter 注册抓取查看路由 /_dump/。
// 鉴权：必须是 root 角色登录会话（cookie session 中 role >= RoleRootUser）。
// 不满足 -> GET 页面跳转到 /login 让你登录，API 调用直接 404，不暴露路由存在。
func SetChatDumpRouter(router *gin.Engine) {
	g := router.Group("/_dump")
	g.Use(chatDumpAuth)
	{
		g.GET("/", chatDumpIndex)
		g.GET("/list", chatDumpList)
		g.GET("/file/:date/:name", chatDumpFile)
		g.GET("/export", chatDumpExport)
		g.DELETE("/file/:date/:name", chatDumpDelete)

		// 验收数据集相关
		g.GET("/dataset/preview", chatDumpDatasetPreview)
		g.GET("/dataset/stats", chatDumpDatasetStats)
		g.GET("/dataset.zip", chatDumpDatasetZip)
	}
	common.SysLog("chatdump: 已注册查看路由 /_dump/（需 root 登录）")
}

// chatDumpAuth 仅放行 root 登录会话。
func chatDumpAuth(c *gin.Context) {
	session := sessions.Default(c)
	idVal := session.Get("id")
	roleVal := session.Get("role")
	statusVal := session.Get("status")

	authed := false
	if idVal != nil && roleVal != nil {
		if role, ok := roleVal.(int); ok && role >= common.RoleRootUser {
			if status, ok := statusVal.(int); !ok || status != common.UserStatusDisabled {
				authed = true
			}
		}
	}
	if authed {
		c.Next()
		return
	}
	// 浏览器直接打开页面：跳到登录后回到这里
	if c.Request.Method == http.MethodGet && strings.HasSuffix(c.Request.URL.Path, "/_dump/") {
		c.Redirect(http.StatusFound, "/login?expired=true")
		return
	}
	c.AbortWithStatus(http.StatusNotFound)
}

func chatDumpList(c *gin.Context) {
	filter := chatdump.ListFilter{
		Date:  c.Query("date"),
		Model: c.Query("model"),
	}
	offset := 0
	if v, err := strconv.Atoi(c.Query("offset")); err == nil && v > 0 {
		offset = v
	}
	limit := 200
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 {
		limit = v
		if limit > 1000 {
			limit = 1000
		}
	}
	files, total, err := chatdump.ListFilesPaged(filter, offset, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp := gin.H{
		"enabled": chatdump.IsEnabled(),
		"root":    chatdump.DumpRoot(),
		"files":   files,
		"total":   total,
		"offset":  offset,
		"limit":   limit,
	}
	// 日期下拉只在首屏返回，翻页时省掉一次目录扫描
	if offset == 0 {
		resp["dates"], _ = chatdump.ListDates()
	}
	c.JSON(http.StatusOK, resp)
}

func chatDumpFile(c *gin.Context) {
	data, err := chatdump.ReadFile(c.Param("date"), c.Param("name"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", data)
}

func chatDumpDelete(c *gin.Context) {
	if err := chatdump.DeleteFile(c.Param("date"), c.Param("name")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func chatDumpExport(c *gin.Context) {
	filter := chatdump.ListFilter{
		Date:  c.Query("date"),
		Model: c.Query("model"),
	}
	var buf bytes.Buffer
	if err := chatdump.WriteZip(filter, &buf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	fname := "chatdump-" + time.Now().Format("20060102-150405") + ".zip"
	c.Header("Content-Disposition", `attachment; filename="`+fname+`"`)
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

func chatDumpIndex(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(chatDumpIndexHTML))
}

// 解析查询参数构建 ExportOptions。
func parseExportOptions(c *gin.Context) chatdump.ExportOptions {
	opts := chatdump.DefaultExportOptions()
	if v := c.Query("min_turns"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			opts.MinTurns = n
		}
	}
	if v := c.Query("min_thinking_budget"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			opts.MinThinkingBudget = n
		}
	}
	if v := c.Query("min_reasoning_ratio"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			opts.MinReasoningRatio = n
		}
	}
	if v := c.Query("require_thinking"); v != "" {
		opts.RequireThinking = v == "1" || strings.EqualFold(v, "true")
	}
	opts.DateFilter = c.Query("date")
	opts.ModelFilter = c.Query("model")
	return opts
}

// 仅返回统计，不返回 trajectory 数据。
func chatDumpDatasetStats(c *gin.Context) {
	opts := parseExportOptions(c)
	trajs, stats, err := chatdump.BuildDataset(opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	domainCount := map[string]int{}
	scaffoldCount := map[string]int{}
	for _, t := range trajs {
		domainCount[t.Metadata.Domain]++
		scaffoldCount[t.Metadata.Scaffold]++
	}
	c.JSON(http.StatusOK, gin.H{
		"options":  opts,
		"stats":    stats,
		"domains":  domainCount,
		"scaffold": scaffoldCount,
	})
}

// 预览前 N 条转换结果（默认 3）。
func chatDumpDatasetPreview(c *gin.Context) {
	opts := parseExportOptions(c)
	n := 3
	if v := c.Query("n"); v != "" {
		if x, err := strconv.Atoi(v); err == nil && x > 0 && x <= 50 {
			n = x
		}
	}
	trajs, stats, err := chatdump.BuildDataset(opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n > len(trajs) {
		n = len(trajs)
	}
	c.JSON(http.StatusOK, gin.H{
		"stats":   stats,
		"preview": trajs[:n],
	})
}

// 整包导出 zip，每条 trajectory 一个 JSON，外加 _stats.json / _index.csv。
func chatDumpDatasetZip(c *gin.Context) {
	opts := parseExportOptions(c)
	trajs, stats, err := chatdump.BuildDataset(opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// _stats.json
	if w, err := zw.Create("_stats.json"); err == nil {
		domainCount := map[string]int{}
		scaffoldCount := map[string]int{}
		for _, t := range trajs {
			domainCount[t.Metadata.Domain]++
			scaffoldCount[t.Metadata.Scaffold]++
		}
		b, _ := json.MarshalIndent(map[string]any{
			"options":  opts,
			"stats":    stats,
			"domains":  domainCount,
			"scaffold": scaffoldCount,
			"exported": len(trajs),
		}, "", "  ")
		_, _ = w.Write(b)
	}

	// _index.csv
	if w, err := zw.Create("_index.csv"); err == nil {
		_, _ = w.Write([]byte("file,model,thinking,turns,reasoning_ratio,domain,scaffold,user_id\n"))
		for i, t := range trajs {
			fname := fmt.Sprintf("%s/%04d_%s_%s_u%d.json",
				safe(t.Metadata.Domain), i+1, safe(t.ModelConfig.Model),
				safe(t.Metadata.Scaffold), t.Metadata.UserID)
			row := fmt.Sprintf("%s,%s,%s,%d,%.2f,%s,%s,%d\n",
				fname, t.ModelConfig.Model, t.ModelConfig.Thinking,
				t.Metadata.Turns, t.Metadata.ReasoningRatio,
				t.Metadata.Domain, t.Metadata.Scaffold, t.Metadata.UserID)
			_, _ = w.Write([]byte(row))
		}
	}

	for i, t := range trajs {
		fname := fmt.Sprintf("%s/%04d_%s_%s_u%d.json",
			safe(t.Metadata.Domain), i+1, safe(t.ModelConfig.Model),
			safe(t.Metadata.Scaffold), t.Metadata.UserID)
		w, err := zw.Create(fname)
		if err != nil {
			continue
		}
		b, err := json.MarshalIndent(t, "", "  ")
		if err != nil {
			continue
		}
		_, _ = w.Write(b)
	}

	if err := zw.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	out := "dataset-" + time.Now().Format("20060102-150405") + ".zip"
	c.Header("Content-Disposition", `attachment; filename="`+out+`"`)
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

func safe(s string) string {
	if s == "" {
		return "unknown"
	}
	r := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	return r.Replace(s)
}

// 单页 HTML，纯原生 JS + fetch，相对路径访问同前缀下的 API。
const chatDumpIndexHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width,initial-scale=1" />
<meta name="robots" content="noindex,nofollow" />
<title>Chat Dump</title>
<style>
  :root { color-scheme: dark; }
  * { box-sizing: border-box; }
  body { margin: 0; font: 13px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background:#0d1117; color:#c9d1d9; }
  header { display:flex; gap:12px; align-items:center; padding:10px 14px; background:#161b22; border-bottom:1px solid #30363d; flex-wrap:wrap; }
  header h1 { margin:0; font-size:14px; color:#58a6ff; }
  header input, header select, header button { background:#0d1117; color:#c9d1d9; border:1px solid #30363d; padding:6px 8px; border-radius:6px; font:inherit; }
  header button { cursor:pointer; }
  header button:hover { background:#1f6feb; border-color:#1f6feb; }
  header .stat { color:#8b949e; margin-left:auto; }
  .layout { display:grid; grid-template-columns: 360px 1fr; height: calc(100vh - 49px); }
  .list { overflow:auto; border-right:1px solid #30363d; }
  .row { padding:10px 14px; border-bottom:1px solid #21262d; cursor:pointer; }
  .row:hover { background:#161b22; }
  .row.active { background:#1f2937; }
  .row .model { color:#79c0ff; font-weight:600; word-break:break-all; }
  .row .meta { color:#8b949e; font-size:11px; margin-top:2px; }
  .row .del { float:right; color:#f85149; opacity:.0; }
  .row:hover .del { opacity:.7; }
  .row .del:hover { opacity:1; }
  .detail { overflow:auto; padding:14px 18px; }
  .detail h2 { font-size:13px; color:#58a6ff; margin: 18px 0 6px; border-bottom:1px solid #30363d; padding-bottom:4px; }
  .detail h2:first-child { margin-top:0; }
  .kv { display:grid; grid-template-columns: 140px 1fr; row-gap:4px; column-gap:12px; }
  .kv b { color:#8b949e; font-weight:normal; }
  pre { background:#161b22; border:1px solid #30363d; border-radius:6px; padding:10px; overflow:auto; white-space:pre-wrap; word-break:break-all; max-height:60vh; }
  pre.short { max-height:28vh; }
  .pill { display:inline-block; padding:1px 6px; border-radius:10px; background:#1f6feb; color:#fff; font-size:11px; margin-right:4px; }
  .empty { padding:40px; text-align:center; color:#8b949e; }
  .chain { font-style:italic; color:#d2a8ff; }
</style>
</head>
<body>
<header>
  <h1>Chat Dump</h1>
  <select id="date"></select>
  <input id="model" placeholder="模型关键字" />
  <button onclick="load()">刷新</button>
  <button onclick="exportZip()">导出原始 zip</button>
  <span style="border-left:1px solid #30363d;padding-left:10px;color:#8b949e">验收:</span>
  <input id="ds_min_turns" type="number" value="5" title="最小总轮次" style="width:60px" />
  <input id="ds_min_budget" type="number" value="4096" title="thinking 预算阈值" style="width:80px" />
  <input id="ds_min_ratio" type="number" step="0.1" value="0.5" title="reasoning 最小占比" style="width:60px" />
  <label style="color:#8b949e"><input id="ds_require_thinking" type="checkbox" checked /> 必须 thinking</label>
  <button onclick="datasetStats()">统计</button>
  <button onclick="datasetPreview()">预览</button>
  <button onclick="datasetZip()" style="background:#238636;border-color:#238636">导出验收 zip</button>
  <span class="stat" id="stat"></span>
</header>
<div class="layout">
  <div class="list" id="list"></div>
  <div class="detail" id="detail"><div class="empty">点击左侧条目查看详情</div></div>
</div>
<script>
const BASE = location.pathname.replace(/\/$/, '');
const PAGE = 200;            // 每页条数
const SCROLL_PAD = 300;      // 距底部多少 px 时预加载下一页
let items = [];              // 已加载的全部条目（累积）
let total = 0;               // 当前筛选下的总条数
let offset = 0;              // 下一页起始位置
let loading = false;
let current = null;
let activeEl = null;

const listEl = () => document.getElementById('list');

// 重新加载：筛选条件变化 / 首屏 / 删除后调用。
async function load() {
  items = []; total = 0; offset = 0; current = null; activeEl = null;
  listEl().innerHTML = '<div class="empty" id="more">加载中…</div>';
  await fetchPage(true);
}

async function fetchPage(isFirst) {
  if (loading) return;
  loading = true;
  setMore('加载中…', false);
  const date = document.getElementById('date').value || '';
  const model = document.getElementById('model').value.trim();
  const q = new URLSearchParams();
  if (date) q.set('date', date);
  if (model) q.set('model', model);
  q.set('offset', offset);
  q.set('limit', PAGE);
  let j;
  try {
    const r = await fetch(BASE + '/list?' + q.toString());
    j = await r.json();
  } catch (e) {
    setMore('加载失败，点此重试', true);
    loading = false;
    return;
  }
  const page = j.files || [];
  total = j.total || 0;
  offset += page.length;

  if (isFirst) {
    document.getElementById('stat').textContent =
      (j.enabled ? '已启用' : '未启用') + ' · ' + j.root + ' · 共 ' + total + ' 条';
    // 日期下拉只在首屏返回，避免翻页时重建
    if (j.dates) {
      const sel = document.getElementById('date');
      const cur = sel.value;
      sel.innerHTML = '<option value="">全部日期</option>' +
        j.dates.map(d => '<option value="' + d + '"' + (d === cur ? ' selected' : '') + '>' + d + '</option>').join('');
    }
  }

  const startIdx = items.length;
  items = items.concat(page);
  appendRows(page, startIdx);
  updateMore();
  loading = false;
}

// 只把新一页拼成 HTML 追加到列表尾部，不重建已有 DOM。
function appendRows(page, startIdx) {
  const html = page.map((f, k) => rowHtml(f, startIdx + k)).join('');
  const moreEl = document.getElementById('more');
  if (moreEl) moreEl.insertAdjacentHTML('beforebegin', html);
  else listEl().insertAdjacentHTML('beforeend', html);
}

function rowHtml(f, i) {
  const parts = f.name.replace(/\.json$/, '').split('_');
  const ts = parts[0] || '';
  const model = parts.slice(1, -2).join('_');
  const userPart = parts[parts.length - 2] || '';
  return '<div class="row" data-i="' + i + '">' +
    '<span class="del">删除</span>' +
    '<div class="model">' + escapeHtml(model || f.name) + '</div>' +
    '<div class="meta">' + f.date + ' ' + formatTime(ts) + ' · ' + userPart + ' · ' + f.size_kb + ' KB</div>' +
    '</div>';
}

// 列表底部的状态/加载更多哨兵。
function setMore(text, retry) {
  let m = document.getElementById('more');
  if (!m) {
    m = document.createElement('div');
    m.id = 'more';
    m.className = 'empty';
    listEl().appendChild(m);
  }
  m.textContent = text;
  m.onclick = retry ? () => fetchPage(items.length === 0) : null;
  m.style.cursor = retry ? 'pointer' : 'default';
}

function updateMore() {
  if (!items.length) { setMore('没有匹配的数据', false); return; }
  if (items.length < total) setMore('下拉加载更多 · 已加载 ' + items.length + ' / ' + total, false);
  else setMore('已全部加载 · 共 ' + items.length + ' 条', false);
}

// 事件委托：整个列表只挂一个 click 监听器，避免给几万行各挂一个。
function initList() {
  const el = listEl();
  el.addEventListener('click', e => {
    const row = e.target.closest('.row');
    if (!row) return;
    const i = +row.dataset.i;
    if (e.target.classList.contains('del')) { del(i); return; }
    if (activeEl) activeEl.classList.remove('active');
    row.classList.add('active');
    activeEl = row;
    show(items[i]);
  });
  // 滚动到底自动加载下一页。
  el.addEventListener('scroll', () => {
    if (loading || items.length >= total) return;
    if (el.scrollTop + el.clientHeight >= el.scrollHeight - SCROLL_PAD) fetchPage(false);
  });
}

function formatTime(s) {
  // HHMMSSmmm
  if (!/^\d{9}$/.test(s)) return s;
  return s.slice(0, 2) + ':' + s.slice(2, 4) + ':' + s.slice(4, 6) + '.' + s.slice(6);
}

async function show(f) {
  current = f;
  const r = await fetch(BASE + '/file/' + encodeURIComponent(f.date) + '/' + encodeURIComponent(f.name));
  const rec = await r.json();
  const el = document.getElementById('detail');

  const kv = [
    ['时间', rec.timestamp],
    ['上游模型', rec.upstream_model],
    ['原始模型', rec.origin_model],
    ['流式', rec.stream ? '是' : '否'],
    ['耗时(ms)', rec.duration_ms],
    ['用户', rec.user_id + (rec.user_email ? ' / ' + rec.user_email : '')],
    ['Token', rec.token_id],
    ['渠道', rec.channel_id + ' (type=' + rec.channel_type + ')'],
    ['IP', rec.ip],
    ['UA', rec.user_agent],
    ['路径', rec.request_path],
    ['Relay 格式', rec.relay_format],
    ['Dump ID', rec.dump_id],
  ];
  if (rec.error) kv.push(['错误', rec.error]);

  let html = '<h2>元信息</h2><div class="kv">' +
    kv.map(([k, v]) => '<b>' + k + '</b><span>' + (v == null ? '' : escapeHtml(String(v))) + '</span>').join('') +
    '</div>';

  if (rec.aggregated_thinking) {
    html += '<h2>思考链 (aggregated)</h2><pre class="chain">' + escapeHtml(rec.aggregated_thinking) + '</pre>';
  }
  if (rec.aggregated_text) {
    html += '<h2>模型回复 (aggregated)</h2><pre>' + escapeHtml(rec.aggregated_text) + '</pre>';
  }
  if (rec.usage) {
    html += '<h2>Usage</h2><pre class="short">' + escapeHtml(JSON.stringify(rec.usage, null, 2)) + '</pre>';
  }
  if (rec.request) {
    html += '<h2>请求体（完整参数 / system / messages / tools / thinking）</h2><pre>' + escapeHtml(JSON.stringify(rec.request, null, 2)) + '</pre>';
  }
  if (rec.response) {
    html += '<h2>响应体</h2><pre>' + escapeHtml(JSON.stringify(rec.response, null, 2)) + '</pre>';
  }
  if (rec.stream_events && rec.stream_events.length) {
    html += '<h2>流式事件（' + rec.stream_events.length + ' 条）</h2><pre>' +
      escapeHtml(rec.stream_events.map(e => '[' + e.t_ms + 'ms] ' + JSON.stringify(e.data)).join('\n')) +
      '</pre>';
  }
  el.innerHTML = html;
}

async function del(i) {
  if (!confirm('删除这条记录？')) return;
  const f = items[i];
  await fetch(BASE + '/file/' + encodeURIComponent(f.date) + '/' + encodeURIComponent(f.name), { method: 'DELETE' });
  load();
}

function exportZip() {
  location.href = BASE + '/export?' + datasetQuery(false);
}

function datasetQuery(includeThresholds) {
  const q = new URLSearchParams();
  const date = document.getElementById('date').value || '';
  const model = document.getElementById('model').value.trim();
  if (date) q.set('date', date);
  if (model) q.set('model', model);
  if (includeThresholds) {
    q.set('min_turns', document.getElementById('ds_min_turns').value || '5');
    q.set('min_thinking_budget', document.getElementById('ds_min_budget').value || '4096');
    q.set('min_reasoning_ratio', document.getElementById('ds_min_ratio').value || '0.5');
    q.set('require_thinking', document.getElementById('ds_require_thinking').checked ? '1' : '0');
  }
  return q.toString();
}

async function datasetStats() {
  const r = await fetch(BASE + '/dataset/stats?' + datasetQuery(true));
  const j = await r.json();
  const el = document.getElementById('detail');
  el.innerHTML = '<h2>验收数据集统计</h2>' +
    '<pre>' + escapeHtml(JSON.stringify(j, null, 2)) + '</pre>';
}

async function datasetPreview() {
  const r = await fetch(BASE + '/dataset/preview?n=3&' + datasetQuery(true));
  const j = await r.json();
  const el = document.getElementById('detail');
  let html = '<h2>预览（前 3 条）</h2><pre class="short">' +
    escapeHtml(JSON.stringify(j.stats, null, 2)) + '</pre>';
  (j.preview || []).forEach((t, i) => {
    html += '<h2>#' + (i+1) + ' · ' + escapeHtml(t.metadata.domain || 'unknown') +
      ' · ' + escapeHtml(t.metadata.scaffold || '') +
      ' · ' + t.metadata.turns + ' 轮 · reasoning ' + (t.metadata.reasoning_ratio*100|0) + '%' +
      '</h2><pre>' + escapeHtml(JSON.stringify(t, null, 2)) + '</pre>';
  });
  el.innerHTML = html;
}

function datasetZip() {
  location.href = BASE + '/dataset.zip?' + datasetQuery(true);
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

initList();
load();
</script>
</body>
</html>`
