# 更新 .env 配置

在你的 `.env` 文件中添加 R2 公开域名：

```bash
# Cloudflare R2 配置
R2_ACCOUNT_ID=704aae0a719434517c177925d555c107
R2_BUCKET_NAME=dexter-media-resources
R2_ACCESS_KEY_ID=你的访问密钥ID
R2_SECRET_ACCESS_KEY=你的访问密钥Secret
R2_REGION=auto
R2_VIDEO_UPLOAD_ENABLED=true
R2_VIDEO_EXPIRY_DAYS=2

# 添加这一行 - 使用 R2 公开开发域名
R2_PUBLIC_DOMAIN=pub-d00a765d53c94c4fb7839204dc1c6aaa.r2.dev
```

## 说明

添加 `R2_PUBLIC_DOMAIN` 后，生成的视频 URL 将是：
```
https://pub-d00a765d53c94c4fb7839204dc1c6aaa.r2.dev/videos/2026-03-08/test_xxx.mp4
```

这个 URL 可以直接在浏览器中访问，无需额外配置。

## 测试

添加配置后，重新运行测试：
```bash
go run test_r2_upload.go
```

你应该会看到使用新域名的 URL。
