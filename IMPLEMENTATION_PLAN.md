# 接入 Vidu S1 数字人（非实时 · 视频文件版）

## 背景

上游有两条数字人产品线，形状完全不同：

- **实时交互**（`POST /live/v1/lives` + WebSocket 控制链 + 阿里 RTC 媒体流）：只有建会话那一下经过 hub，音视频和控制信令客户端直连上游，计费按会话秒数事后回查。hub 发出 RTC 凭据后就失去管控，无法按量计费/限流/中断。**本次不接。**
- **非实时 · 视频文件版**（`POST /ent/v2/avatar-video/generations`）：标准异步任务，轮询端点与 hub 现有 vidu 渠道完全相同（`/ent/v2/tasks/{id}/creations`）。**本次接这条。**

额度现状（2026-09-15 实测）：当前 key 只买了 `task_type: live`，avatar-video 报 `CreditInsufficient`。代码先写好，额度到位后做上游实测验收。

## 对客契约

```
POST /v1/video/generations
{
  "model": "vidu-s1-540p",                 // 或 vidu-s1-720p
  "image": "https://.../portrait.png",     // 人像图，必填
  "prompt": "大家好，欢迎光临",              // 要说的话（文本模式）
  "metadata": {
    "audio": "https://.../speech.mp3",     // 给音频则优先于 prompt
    "voice": "<voice_id>",                 // 音色（文本模式可选）
    "timeline": "{\"actions\":[...]}"      // 动作时间轴（可选）
  }
}
```

分辨率只由模型名后缀决定，不接受请求参数覆盖 —— 与 seedance 同样的理由：上游按分辨率计价，放开会让低价档模型成本超过售价。

---

## Stage 1: 请求链路
**Goal**: 数字人模型的提交请求正确打到 `/ent/v2/avatar-video/generations`，字段映射正确，分辨率锁死。
**Success Criteria**:
- 模型名识别数字人路径，不影响现有四个常规 vidu 端点的路由
- `image` / `prompt`→`text` / `metadata.audio` / `voice` / `timeline` 映射正确
- audio 与 text 同时给出时，audio 优先（与上游一致）
- 模型名后缀锁死 `resolution`，metadata 里的 resolution 被忽略
**Tests**: 路由分流、字段映射、audio 优先、分辨率锁死不被覆盖
**Status**: Complete

## Stage 2: 纯音频模式放行
**Goal**: 只给 audio 不给 prompt 的请求不被公共校验拦掉。
**Success Criteria**: 公共 `validatePrompt` 对"带音频输入的数字人请求"豁免，其他模型行为不变
**Tests**: 纯音频请求通过、纯文本请求通过、两者都缺时仍报错
**Status**: Complete

## Stage 3: 计费
**Goal**: 按视频秒数计费，提交时估得准，提交后用上游实际返回校正。
**Success Criteria**:
- audio 模式：探测音频真实时长（复用 `common.GetAudioDuration`），向上取整
- text 模式：按文本估算（中日韩按字、拉丁按词），估高不估低，避免预扣不足
- `AdjustBillingOnSubmit` 用上游响应的 `credits` 精确校正
**Tests**: 文本估时长（中/英/混合）、上游 credits 校正、缺字段时不炸
**Status**: Complete

## Stage 4: 验收（待额度到位）
**Goal**: 上游真跑一单，确认计费口径与产物可用。
**Success Criteria**:
- 提交→轮询→拿到视频 URL 全链路通
- **确认 `credits` 与视频秒数的换算关系**（文档称 1 秒 = 1 credit，未实测）
- 定价拍板后写入默认价
**Status**: Blocked — 等非实时额度到账

上游形状已做零成本验证（额度检查之前的校验全部通过）：`image` / `audio` / `text` /
`voice` / `timeline` 字段名与必填项无误。但 `resolution` 传 `1080p` 也没被拦 ——
取值校验发生在额度检查之后，说明上游不会替我们兜住分辨率，模型名钉死是唯一防线。
