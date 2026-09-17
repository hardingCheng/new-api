# 接入音视频超分增强（腾讯云 MPS 大模型增强 / 修复）

## 背景

上游是腾讯云媒体处理（MPS）的「音视频增强」能力：给一段已有视频，输出画质增强后的更高分辨率
版本。接口只有两个：`ProcessMedia` 提交、`DescribeTaskDetail` 查询，签名 TC3-HMAC-SHA256，
出错时 HTTP 仍是 200、错误放在 `Response.Error`。

上游计费 = 极速高清转码 + 大模型增强（或修复），三个维度都按**输出**算：分辨率（模板钉死）、
帧率档（≤30 / ≤60 / ≤120 帧，1× / 2× / 4×）、时长（随源）。

## 对客契约

```
POST /v1/video/generations
{
  "model": "video-enhance-1080p",            // video-enhance-{720p|1080p|2k|4k} / video-restore-{...}
  "input_reference": "https://.../in.mp4",   // 源视频公网地址（也认 metadata.video_url / content[].video_url）
  "seconds": "120",                          // 源视频时长，必填，用于预扣
  "metadata": { "scene": "anime" }           // 可选：real（默认）/ anime / real-small-face / anime-small-face / seedance
}
GET /v1/video/generations/{id}               // 完成后 url = 上游临时链接，7 天有效
```

- 分辨率与能力系列只由模型名决定（定价维度），场景只选模板、不改价。
- 模型名必须是上面 8 个之一；渠道模型映射改的是上游名，不参与解析。
- 渠道密钥框：`SecretId|SecretKey|Region|Bucket`。输出走上游临时链接，但上游仍要求填一个
  账号下真实存在的 COS 桶过校验。

## 计费口径

- 预扣：模型单价（元/秒）× 调用方声明的秒数，帧率按 ≤30 帧档估。
- 完成后：用上游回报的输出 `Duration`（进位到秒）与 `Fps` 档位重算，差额补扣或退还
  （`AdjustBillingOnComplete`）。帧率档位倍数作用于整价，比上游只对增强部分翻倍略保守。
- 失败：走框架统一退款。

---

## Stage 1: 渠道类型与适配器
**Goal**: 新渠道类型 63，提交 / 轮询 / 结算三段跑通，密钥不落请求体。
**Success Criteria**:
- 模型名 → 模板 ID / 短边；场景选模板；修复系列忽略场景
- 源地址三种来源取值，非 http(s) 拒绝；秒数缺失 / 自动 / 越界拒绝
- ProcessMedia 请求体：URL 输入、COS 校验桶、StdExtInfo 带 short 与 use_internal_cos
- 签名规范请求串与官方算例哈希一致
- 轮询：WAITING / PROCESSING（进度封顶 99%）/ FINISH 成功取 tmpl_url / FINISH 失败取原因；
  永久性错误判失败、鉴权限流类错误留待下一轮
- 完成后按实际秒数 × 帧率档重算额度
**Tests**: `relay/channel/task/tencentmps` 18 条单测
**Status**: Complete

## Stage 2: 部署 + 建渠道 + 配价
**Goal**: 三站可下单。
**Success Criteria**:
- 分支 rebase 到生产 HEAD 后构建镜像部署，容器健康
- 控制台侧：开通 MPS、授权 COS、建一个 COS 桶、拿 SecretId/SecretKey
- 渠道挂「视频分组」（倍率 1），8 个模型配单价；脚本 `~/ai/hub-mps-enhance-20260917.sh`
**Status**: Complete — 2026-09-17 部署 cheng:mps-enhance-20260917;2026-09-18 渠道 98 建成、建议价生效

## Stage 3: 真跑验收
**Goal**: 上游真跑一单，确认口径。
**Success Criteria**:
- 预制模板 3270xx 在本账号可用：24 个全部可见，h264、帧率随源 ✓
- 直连上游：720p 24 帧 5.06 秒 → 1080p，两个模板均成功，终态响应形状与适配器一致，已做脱敏样本 ✓
- 经 hub 对客链路：提交 200 → 轮询 76 秒到 SUCCESS → 临时链接可下载 1920x1080 ✓
- 计费：声明 5 秒预扣 ¥0.40，完成后按上游 5.062 秒进位 6 秒补扣 ¥0.08，任务终额 ¥0.48，与设计一致 ✓
- 未验：60 帧源是否落 2× 档（需要一段 60 帧源视频）；月底对上游账单
**Status**: Complete（60 帧档位留待有素材时补验）

## 遗留（上一项）

Vidu 数字人 Stage 4（真跑验收、确认积分↔秒换算）仍卡在上游非实时额度未到账，见记忆。
