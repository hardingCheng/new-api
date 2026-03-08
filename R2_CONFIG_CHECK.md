# R2 配置检查

根据你提供的 S3 API 端点：
`https://704aae0a719434517c177925d555c107.r2.cloudflarestorage.com/dexter-media-resources`

## 你的 .env 文件应该这样配置：

```bash
# Cloudflare R2 配置
R2_ACCOUNT_ID=704aae0a719434517c177925d555c107
R2_BUCKET_NAME=dexter-media-resources
R2_ACCESS_KEY_ID=你的访问密钥ID
R2_SECRET_ACCESS_KEY=你的访问密钥Secret
R2_REGION=auto
R2_VIDEO_UPLOAD_ENABLED=true
R2_VIDEO_EXPIRY_DAYS=2
# R2_PUBLIC_DOMAIN=videos.yourdomain.com  # 可选
```

## 重要说明：

1. **R2_ACCOUNT_ID** = `704aae0a719434517c177925d555c107`（从端点 URL 中提取）
2. **R2_BUCKET_NAME** = `dexter-media-resources`（你的 bucket 名称）
3. **R2_ACCESS_KEY_ID** 和 **R2_SECRET_ACCESS_KEY** 是你在 Cloudflare 创建的 API 令牌

## 测试步骤：

1. 确认 `.env` 文件配置正确
2. 运行测试：`go run test_r2_upload.go`
3. 检查是否成功上传

## 预期结果：

上传成功后，文件 URL 格式应该是：
- 使用默认域名：`https://dexter-media-resources.704aae0a719434517c177925d555c107.r2.cloudflarestorage.com/videos/2026-03-08/test_xxx.mp4`
- 或使用自定义域名（如果配置了 R2_PUBLIC_DOMAIN）

## 如果还有问题：

请提供完整的错误信息，我会继续帮你调试。
