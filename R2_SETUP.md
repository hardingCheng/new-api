# Cloudflare R2 视频存储配置指南

## ✅ 已完成配置

你的 R2 配置已经设置完成：

```bash
R2_ACCOUNT_ID=MafA1GHxL4LJAZdVwD4hGB8VYsCNsT8GIMLcz1iM
R2_BUCKET_NAME=sora2videos
R2_ACCESS_KEY_ID=ad38c64eb2bdbeaa9e9a91c166fb8056
R2_SECRET_ACCESS_KEY=4f8c8dde290c2a173e65b3328398590819cbc4e414bcc44a885403d7eb37df3c
R2_REGION=auto
R2_VIDEO_UPLOAD_ENABLED=true
R2_VIDEO_EXPIRY_DAYS=2
```

## 🚀 快速测试

### 1. 测试 R2 上传功能

```bash
go run test_r2_upload.go
```

预期输出：
```
=== Cloudflare R2 Upload Test ===

R2 Configuration:
  Enabled: true
  Expiry Days: 2

✓ R2 uploader initialized successfully

Test Parameters:
  Source URL: https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/BigBuckBunny.mp4
  Task ID: test_1709884800
  Object Key: videos/2026-03-08/test_1709884800.mp4

Starting upload...

✓ Upload successful!
  Duration: 8.5s
  R2 URL: https://sora2videos.MafA1GHxL4LJAZdVwD4hGB8VYsCNsT8GIMLcz1iM.r2.cloudflarestorage.com/videos/2026-03-08/test_1709884800.mp4

✓ Cleanup test completed

=== Test Complete ===
```

### 2. 启用 R2 公开访问

为了让视频 URL 可以公开访问，需要在 Cloudflare Dashboard 中启用：

1. 访问 https://dash.cloudflare.com/
2. 进入 **R2** → 选择桶 **sora2videos**
3. 点击 **Settings** 标签
4. 在 **Public access** 部分：
   - 点击 **Allow Access** 按钮
   - 或者配置自定义域名（推荐）

### 3. 配置自定义域名（可选但推荐）

使用自定义域名可以：
- 更短更友好的 URL
- 更好的品牌形象
- 更快的访问速度（通过 Cloudflare CDN）

步骤：
1. 在 R2 桶设置中，点击 **Connect Domain**
2. 输入你的域名（例如：`videos.yourdomain.com`）
3. 按照提示添加 DNS 记录
4. 更新 `.env` 文件：
   ```bash
   R2_PUBLIC_DOMAIN=videos.yourdomain.com
   ```

### 4. 启动应用

```bash
go build -o new-api
./new-api
```

查看启动日志，确认看到：
```
R2 uploader initialized: account=MafA1GHxL4LJAZdVwD4hGB8VYsCNsT8GIMLcz1iM, bucket=sora2videos, expiry=2 days
R2 video cleanup task scheduled (expiry: 2 days)
```

## 📊 工作流程

1. **用户提交视频任务** → 调用 `/v1/videos`
2. **系统转发到上游** → OpenAI Sora
3. **任务完成** → 上游返回视频 URL
4. **自动下载** → 从上游 URL 下载视频
5. **上传到 R2** → 上传到 `videos/2026-03-08/task_xxx.mp4`
6. **返回 R2 URL** → 用户获得 R2 URL
7. **自动清理** → 2 天后自动删除

## 🎯 R2 优势

### 相比上游直接 URL
- ✅ **永久存储** - 不受上游 URL 过期限制
- ✅ **访问控制** - 可以自己控制访问权限
- ✅ **成本优化** - 零出站费用
- ✅ **全球加速** - Cloudflare CDN 加速

### 相比其他对象存储
- ✅ **零出站费用** - 下载流量完全免费
- ✅ **S3 兼容** - 无需修改代码
- ✅ **全球 CDN** - 自动分发
- ✅ **价格便宜** - $0.015/GB/月
- ✅ **免费额度** - 每月 10GB 免费

## 📈 成本估算

假设每天生成 100 个视频，每个视频 50MB：

### 存储成本
- 每天：100 × 50MB = 5GB
- 2 天保留：5GB × 2 = 10GB
- 月成本：10GB × $0.015 = **$0.15/月**

### 流量成本
- R2 出站流量：**$0**（完全免费）
- 传统 S3：10GB × $0.09 = $0.90/月

### 总成本
- R2：**$0.15/月**
- 传统 S3：$1.05/月
- **节省 86%**

## 🔧 故障排查

### 问题 1: 上传失败

**错误信息：** `failed to upload to R2: ...`

**解决方案：**
1. 检查 Access Key 和 Secret 是否正确
2. 确认桶名是否正确
3. 验证 Account ID 是否正确
4. 检查网络连接

### 问题 2: 视频 URL 无法访问

**错误信息：** `403 Forbidden` 或 `404 Not Found`

**解决方案：**
1. 确认已启用 R2 公开访问
2. 检查文件是否已上传成功
3. 验证 URL 格式是否正确

### 问题 3: 上传速度慢

**可能原因：**
- 视频文件太大
- 网络带宽限制

**解决方案：**
1. 增加超时时间（当前 5 分钟）
2. 检查服务器带宽
3. 考虑使用分片上传（需要修改代码）

## 📝 监控建议

### 关键日志
- `R2 uploader initialized` - R2 初始化成功
- `Video uploaded to R2` - 上传成功
- `Failed to upload video to R2` - 上传失败
- `Cleanup completed` - 清理完成

### 监控指标
- R2 上传成功率
- 上传耗时
- 存储空间使用量
- 清理删除的文件数量

## 🎉 完成

你的 Cloudflare R2 视频存储已经配置完成！

现在可以：
1. ✅ 运行 `go run test_r2_upload.go` 测试上传
2. ✅ 启用 R2 公开访问
3. ✅ 启动应用并提交视频任务
4. ✅ 验证视频 URL 指向 R2
5. ✅ 等待 2 天后验证自动清理

---

**提示**: 从香港服务器访问 Cloudflare R2 速度非常快，延迟通常在 10-30ms 之间。
