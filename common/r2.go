package common

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// R2Config Cloudflare R2 配置
type R2Config struct {
	AccountID       string // Cloudflare 账户 ID
	Bucket          string // 桶名
	AccessKeyID     string // Access Key ID
	AccessKeySecret string // Secret Access Key
	Region          string // 区域，R2 使用 auto
	PublicDomain    string // 可选：自定义公开域名
}

var (
	// R2VideoUploadEnabled 是否启用视频自动上传到 R2
	R2VideoUploadEnabled = false
	// R2VideoExpiryDays 视频文件过期时间（天），0 表示永不过期
	R2VideoExpiryDays = 0
	// globalR2Uploader 全局 R2 上传器
	globalR2Uploader *R2Uploader
)

// InitR2 初始化 R2 配置
func InitR2() {
	R2VideoUploadEnabled = GetEnvOrDefaultBool("R2_VIDEO_UPLOAD_ENABLED", false)
	R2VideoExpiryDays = GetEnvOrDefault("R2_VIDEO_EXPIRY_DAYS", 0)

	SysLog(fmt.Sprintf("R2 initialization: enabled=%v, expiry_days=%d", R2VideoUploadEnabled, R2VideoExpiryDays))

	if !R2VideoUploadEnabled {
		SysLog("R2 video upload is disabled")
		return
	}

	config := R2Config{
		AccountID:       os.Getenv("R2_ACCOUNT_ID"),
		Bucket:          os.Getenv("R2_BUCKET_NAME"),
		AccessKeyID:     os.Getenv("R2_ACCESS_KEY_ID"),
		AccessKeySecret: os.Getenv("R2_SECRET_ACCESS_KEY"),
		Region:          GetEnvOrDefaultString("R2_REGION", "auto"),
		PublicDomain:    os.Getenv("R2_PUBLIC_DOMAIN"),
	}

	if config.AccountID == "" || config.Bucket == "" || config.AccessKeyID == "" || config.AccessKeySecret == "" {
		SysError("R2 configuration is incomplete, video upload to R2 will be disabled")
		R2VideoUploadEnabled = false
		return
	}

	globalR2Uploader = NewR2Uploader(config)
	if R2VideoExpiryDays > 0 {
		SysLog(fmt.Sprintf("R2 uploader initialized: account=%s, bucket=%s, expiry=%d days", config.AccountID, config.Bucket, R2VideoExpiryDays))
	} else {
		SysLog(fmt.Sprintf("R2 uploader initialized: account=%s, bucket=%s, no expiry", config.AccountID, config.Bucket))
	}
}

// GetR2Uploader 获取全局 R2 上传器
func GetR2Uploader() *R2Uploader {
	return globalR2Uploader
}

// R2Uploader Cloudflare R2 上传器
type R2Uploader struct {
	client       *s3.Client
	bucket       string
	accountID    string
	publicDomain string
}

// NewR2Uploader 创建 R2 上传器
func NewR2Uploader(config R2Config) *R2Uploader {
	// R2 endpoint URL - 不包含 bucket 名称
	// 格式：https://<account_id>.r2.cloudflarestorage.com
	r2Endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", config.AccountID)

	// 创建自定义 HTTP 客户端，配置 TLS
	httpClient := &http.Client{
		Timeout: 10 * time.Minute,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				MaxVersion:         tls.VersionTLS13,
				InsecureSkipVerify: false,
			},
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   100,
			IdleConnTimeout:       90 * time.Second,
			DisableKeepAlives:     false,
			DisableCompression:    false,
			ForceAttemptHTTP2:     true,
			MaxConnsPerHost:       0,
			ResponseHeaderTimeout: 30 * time.Second,
		},
	}

	// 创建自定义端点解析函数
	// 关键：使用 HostnameImmutable 防止 SDK 修改主机名
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL:               r2Endpoint,
			HostnameImmutable: true,
			SigningRegion:     "auto",
			Source:            aws.EndpointSourceCustom,
		}, nil
	})

	// 创建 AWS 配置
	cfg := aws.Config{
		Region: config.Region,
		Credentials: credentials.NewStaticCredentialsProvider(
			config.AccessKeyID,
			config.AccessKeySecret,
			"",
		),
		HTTPClient:                  httpClient,
		RetryMaxAttempts:            5,
		RetryMode:                   aws.RetryModeStandard,
		EndpointResolverWithOptions: customResolver,
	}

	// 创建 S3 客户端
	// UsePathStyle=true 确保 URL 格式为：https://endpoint/bucket/key
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	SysLog(fmt.Sprintf("R2 client created with endpoint: %s, bucket: %s", r2Endpoint, config.Bucket))

	return &R2Uploader{
		client:       client,
		bucket:       config.Bucket,
		accountID:    config.AccountID,
		publicDomain: config.PublicDomain,
	}
}

// DownloadAndUpload 从 URL 下载文件并上传到 R2
// 返回 R2 中的文件 URL
func (u *R2Uploader) DownloadAndUpload(ctx context.Context, sourceURL, objectKey string) (string, error) {
	return u.DownloadAndUploadWithAuth(ctx, sourceURL, objectKey, "", "")
}

// UploadData 直接上传数据到 R2
func (u *R2Uploader) UploadData(ctx context.Context, data []byte, objectKey string) (string, error) {
	fileSize := len(data)
	SysLog(fmt.Sprintf("Uploading %d bytes (%.2f MB) to R2...", fileSize, float64(fileSize)/(1024*1024)))

	// 检测 Content-Type
	contentType := http.DetectContentType(data)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// 准备上传参数
	putInput := &s3.PutObjectInput{
		Bucket:        aws.String(u.bucket),
		Key:           aws.String(objectKey),
		Body:          bytes.NewReader(data),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(int64(fileSize)),
	}

	// 如果设置了过期时间，添加元数据
	if R2VideoExpiryDays > 0 {
		expiryTime := time.Now().Add(time.Duration(R2VideoExpiryDays) * 24 * time.Hour)
		putInput.Metadata = map[string]string{
			"expiry-time": expiryTime.Format(time.RFC3339),
			"expiry-days": fmt.Sprintf("%d", R2VideoExpiryDays),
		}
	}

	// 上传到 R2
	SysLog(fmt.Sprintf("Uploading to R2: bucket=%s, key=%s, size=%d bytes", u.bucket, objectKey, fileSize))

	uploadCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	_, err := u.client.PutObject(uploadCtx, putInput)
	if err != nil {
		return "", fmt.Errorf("failed to upload to R2: %w", err)
	}

	// 构建文件 URL
	var fileURL string
	if u.publicDomain != "" {
		fileURL = fmt.Sprintf("https://%s/%s", u.publicDomain, objectKey)
	} else {
		fileURL = fmt.Sprintf("https://%s.%s.r2.cloudflarestorage.com/%s", u.bucket, u.accountID, objectKey)
	}

	SysLog(fmt.Sprintf("Upload successful: %s", fileURL))
	return fileURL, nil
}

// DownloadAndUploadWithProxy 从 URL 下载文件并上传到 R2（支持代理）
// 返回 R2 中的文件 URL
func (u *R2Uploader) DownloadAndUploadWithProxy(ctx context.Context, sourceURL, objectKey, proxyURL string) (string, error) {
	return u.DownloadAndUploadWithAuth(ctx, sourceURL, objectKey, proxyURL, "")
}

// DownloadAndUploadWithAuth 从 URL 下载文件并上传到 R2（支持代理和认证）
// 返回 R2 中的文件 URL
func (u *R2Uploader) DownloadAndUploadWithAuth(ctx context.Context, sourceURL, objectKey, proxyURL, apiKey string) (string, error) {
	// 1. 下载文件
	SysLog(fmt.Sprintf("Downloading video from: %s", sourceURL))

	var fileData []byte
	var err error

	// 对 videos.openai.com 使用 tls-client 下载（模拟 Chrome TLS 指纹绕过 Cloudflare）
	if strings.Contains(sourceURL, "videos.openai.com") {
		SysLog("Using tls-client (Chrome TLS fingerprint) for Cloudflare-protected URL")
		fileData, err = downloadWithTLSClient(ctx, sourceURL, proxyURL)
		if err != nil {
			SysLog(fmt.Sprintf("tls-client failed: %v, falling back to curl", err))
			fileData, err = downloadWithCurl(ctx, sourceURL, proxyURL)
			if err != nil {
				return "", fmt.Errorf("failed to download with both tls-client and curl: %w", err)
			}
		}
	} else {
		// 其他 URL 使用标准 HTTP 客户端
		fileData, err = downloadWithHTTP(ctx, sourceURL, proxyURL, apiKey)
		if err != nil {
			return "", fmt.Errorf("failed to download with HTTP: %w", err)
		}
	}

	fileSize := len(fileData)
	SysLog(fmt.Sprintf("Downloaded %d bytes (%.2f MB), uploading to R2...", fileSize, float64(fileSize)/(1024*1024)))

	// 3. 检测 Content-Type
	contentType := http.DetectContentType(fileData)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// 4. 准备上传参数
	putInput := &s3.PutObjectInput{
		Bucket:        aws.String(u.bucket),
		Key:           aws.String(objectKey),
		Body:          bytes.NewReader(fileData),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(int64(fileSize)),
	}

	// 5. 如果设置了过期时间，添加元数据
	if R2VideoExpiryDays > 0 {
		expiryTime := time.Now().Add(time.Duration(R2VideoExpiryDays) * 24 * time.Hour)
		// 添加自定义元数据记录过期时间
		putInput.Metadata = map[string]string{
			"expiry-time": expiryTime.Format(time.RFC3339),
			"expiry-days": fmt.Sprintf("%d", R2VideoExpiryDays),
		}
	}

	// 6. 上传到 R2
	SysLog(fmt.Sprintf("Uploading to R2: bucket=%s, key=%s, size=%d bytes", u.bucket, objectKey, fileSize))

	// 创建带超时的上下文
	uploadCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	_, err = u.client.PutObject(uploadCtx, putInput)
	if err != nil {
		return "", fmt.Errorf("failed to upload to R2: %w", err)
	}

	// 7. 构建文件 URL
	var fileURL string
	if u.publicDomain != "" {
		// 使用自定义公开域名（R2 Public URL 或自定义域名）
		fileURL = fmt.Sprintf("https://%s/%s", u.publicDomain, objectKey)
	} else {
		// 使用默认 R2 URL（需要开启公开访问）
		// 格式：https://<bucket>.<account_id>.r2.cloudflarestorage.com/<key>
		fileURL = fmt.Sprintf("https://%s.%s.r2.cloudflarestorage.com/%s", u.bucket, u.accountID, objectKey)
	}

	SysLog(fmt.Sprintf("Upload successful: %s", fileURL))

	return fileURL, nil
}

// GenerateObjectKey 生成 R2 对象键
// 格式：videos/{date}/{taskID}.mp4
func GenerateR2ObjectKey(taskID, originalURL string) string {
	// 先去掉查询参数，再提取扩展名
	cleanURL := originalURL
	if idx := strings.IndexByte(cleanURL, '?'); idx != -1 {
		cleanURL = cleanURL[:idx]
	}
	ext := path.Ext(cleanURL)
	if ext == "" {
		ext = ".mp4" // 默认为 mp4
	}

	// 生成日期路径
	date := time.Now().Format("2006-01-02")

	// 生成对象键
	return fmt.Sprintf("videos/%s/%s%s", date, taskID, ext)
}

// CleanupExpiredVideos 清理过期的视频文件
// 这个函数应该由定时任务调用
func CleanupExpiredR2Videos() error {
	if !R2VideoUploadEnabled {
		return nil
	}

	// 如果 R2VideoExpiryDays <= 0，不启用清理（0 表示永不过期）
	if R2VideoExpiryDays <= 0 {
		return nil
	}

	uploader := GetR2Uploader()
	if uploader == nil {
		return fmt.Errorf("R2 uploader not initialized")
	}

	ctx := context.Background()

	// 计算过期日期
	expiryDate := time.Now().Add(-time.Duration(R2VideoExpiryDays) * 24 * time.Hour)
	SysLog(fmt.Sprintf("Starting cleanup of R2 videos older than %s", expiryDate.Format("2006-01-02")))

	// 列出 videos/ 前缀下的所有对象
	listInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(uploader.bucket),
		Prefix: aws.String("videos/"),
	}

	paginator := s3.NewListObjectsV2Paginator(uploader.client, listInput)
	deletedCount := 0
	totalCount := 0

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list objects: %w", err)
		}

		for _, obj := range page.Contents {
			totalCount++
			// 检查对象是否过期
			if obj.LastModified != nil && obj.LastModified.Before(expiryDate) {
				// 删除对象
				_, err := uploader.client.DeleteObject(ctx, &s3.DeleteObjectInput{
					Bucket: aws.String(uploader.bucket),
					Key:    obj.Key,
				})
				if err != nil {
					SysError(fmt.Sprintf("Failed to delete expired object %s: %v", *obj.Key, err))
					continue
				}
				deletedCount++
				SysLog(fmt.Sprintf("Deleted expired video: %s (last modified: %s)", *obj.Key, obj.LastModified.Format(time.RFC3339)))
			}
		}
	}

	if deletedCount > 0 {
		SysLog(fmt.Sprintf("Cleanup completed: deleted %d expired videos (total scanned: %d)", deletedCount, totalCount))
	} else {
		SysLog(fmt.Sprintf("Cleanup completed: no expired videos found (total scanned: %d)", totalCount))
	}

	return nil
}

// ListR2Videos 列出所有视频文件（用于测试和调试）
func ListR2Videos() ([]R2VideoInfo, error) {
	if !R2VideoUploadEnabled {
		return nil, fmt.Errorf("R2 upload is not enabled")
	}

	uploader := GetR2Uploader()
	if uploader == nil {
		return nil, fmt.Errorf("R2 uploader not initialized")
	}

	ctx := context.Background()

	listInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(uploader.bucket),
		Prefix: aws.String("videos/"),
	}

	var videos []R2VideoInfo
	paginator := s3.NewListObjectsV2Paginator(uploader.client, listInput)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}

		for _, obj := range page.Contents {
			if obj.Key != nil && obj.LastModified != nil && obj.Size != nil {
				var fileURL string
				if uploader.publicDomain != "" {
					fileURL = fmt.Sprintf("https://%s/%s", uploader.publicDomain, *obj.Key)
				} else {
					fileURL = fmt.Sprintf("https://%s.%s.r2.cloudflarestorage.com/%s", uploader.bucket, uploader.accountID, *obj.Key)
				}

				videos = append(videos, R2VideoInfo{
					Key:          *obj.Key,
					Size:         *obj.Size,
					LastModified: *obj.LastModified,
					URL:          fileURL,
				})
			}
		}
	}

	return videos, nil
}

// R2VideoInfo 视频文件信息
type R2VideoInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
	URL          string
}

// newTLSClient 创建使用 Chrome TLS 指纹的 tls-client 客户端
// 完美模拟 Chrome 浏览器的 TLS 指纹（JA3/JA4）、HTTP/2 指纹、Header 顺序
func newTLSClient(proxyURL string) (tls_client.HttpClient, error) {
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(600),
		tls_client.WithClientProfile(profiles.Chrome_131),
		tls_client.WithNotFollowRedirects(),
		tls_client.WithInsecureSkipVerify(),
	}

	if proxyURL != "" {
		options = append(options, tls_client.WithProxyUrl(proxyURL))
	}

	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create tls-client: %w", err)
	}

	return client, nil
}

// downloadWithCurl 使用 curl-impersonate 下载文件（完美模拟 Chrome 浏览器）
func downloadWithCurl(ctx context.Context, sourceURL, proxyURL string) ([]byte, error) {
	// 优先使用 curl-impersonate-chrome，如果不存在则回退到普通 curl
	curlCmd := "curl-impersonate-chrome"
	if _, err := exec.LookPath(curlCmd); err != nil {
		SysLog("curl-impersonate not found, falling back to regular curl")
		curlCmd = "curl"
	} else {
		SysLog("Using curl-impersonate-chrome for Cloudflare bypass")
	}

	args := []string{
		"-L",                // 跟随重定向
		"-s",                // 静默模式
		"-S",                // 显示错误
		"--compressed",      // 自动解压
		"--max-time", "600", // 10 分钟超时
	}

	// curl-impersonate 不需要手动设置 headers，它会自动模拟 Chrome
	if curlCmd == "curl" {
		// 普通 curl 需要手动添加 headers
		args = append(args,
			"-H", "User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
			"-H", "Accept: */*",
		)
	}

	if proxyURL != "" {
		args = append(args, "-x", proxyURL)
	}

	args = append(args, sourceURL)

	cmd := exec.CommandContext(ctx, curlCmd, args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			SysError(fmt.Sprintf("curl stderr: %s", string(exitErr.Stderr)))
			return nil, fmt.Errorf("curl failed: %s", string(exitErr.Stderr))
		}
		return nil, err
	}

	if len(output) == 0 {
		return nil, fmt.Errorf("curl returned empty response")
	}

	// 检查是否下载到 HTML 错误页面
	if len(output) < 100000 && bytes.Contains(output, []byte("<html")) {
		SysError(fmt.Sprintf("curl downloaded HTML instead of video (first 500 bytes): %s", string(output[:min(500, len(output))])))
		return nil, fmt.Errorf("curl downloaded HTML error page instead of video")
	}

	SysLog(fmt.Sprintf("curl downloaded %d bytes (%.2f MB)", len(output), float64(len(output))/(1024*1024)))
	return output, nil
}

// downloadWithHTTP 使用标准 HTTP 客户端下载文件
func downloadWithHTTP(ctx context.Context, sourceURL, proxyURL, apiKey string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		SysLog("Using API Key for authentication")
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")

	var client *http.Client
	if proxyURL != "" {
		SysLog(fmt.Sprintf("Using proxy: %s", proxyURL))
		proxyURLParsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse proxy URL: %w", err)
		}
		client = &http.Client{
			Timeout: 10 * time.Minute,
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURLParsed),
			},
		}
	} else {
		client = &http.Client{
			Timeout: 10 * time.Minute,
		}
	}

	SysLog("Sending HTTP request...")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	SysLog(fmt.Sprintf("HTTP response: status=%d, content-length=%d", resp.StatusCode, resp.ContentLength))

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if len(errBody) > 0 {
			SysError(fmt.Sprintf("Download error response body: %s", string(errBody)))
		}
		return nil, fmt.Errorf("status code %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return data, nil
}

// downloadWithTLSClient 使用 tls-client 模拟 Chrome TLS 指纹下载文件
// 纯 Go 实现，不需要浏览器，Docker 镜像轻量
// 完美模拟 Chrome 131 的 JA3/JA4 指纹、HTTP/2 指纹、Header 顺序
func downloadWithTLSClient(ctx context.Context, sourceURL, proxyURL string) ([]byte, error) {
	SysLog("=== Strategy: tls-client (Chrome TLS fingerprint) ===")
	SysLog(fmt.Sprintf("Target URL: %s", sourceURL))

	client, err := newTLSClient(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create tls-client: %w", err)
	}

	// 使用 fhttp 构建请求（tls-client 内部使用 bogdanfinn/fhttp）
	req, err := fhttp.NewRequest("GET", sourceURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header = fhttp.Header{
		"User-Agent":                {"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"},
		"Accept":                    {"*/*"},
		"Accept-Language":           {"en-US,en;q=0.9"},
		"Accept-Encoding":           {"gzip, deflate, br"},
		"Connection":                {"keep-alive"},
		"Sec-Ch-Ua":                 {`"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`},
		"Sec-Ch-Ua-Mobile":          {"?0"},
		"Sec-Ch-Ua-Platform":        {`"Windows"`},
		"Sec-Fetch-Dest":            {"empty"},
		"Sec-Fetch-Mode":            {"cors"},
		"Sec-Fetch-Site":            {"cross-site"},
		"Upgrade-Insecure-Requests": {"1"},
	}

	// tls-client 会自动处理 Header 顺序以匹配 Chrome 指纹
	req.Header[fhttp.HeaderOrderKey] = []string{
		"user-agent",
		"accept",
		"accept-language",
		"accept-encoding",
		"connection",
		"sec-ch-ua",
		"sec-ch-ua-mobile",
		"sec-ch-ua-platform",
		"sec-fetch-dest",
		"sec-fetch-mode",
		"sec-fetch-site",
		"upgrade-insecure-requests",
	}

	SysLog("Sending request with Chrome 131 TLS fingerprint...")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tls-client request failed: %w", err)
	}
	defer resp.Body.Close()

	SysLog(fmt.Sprintf("HTTP response: status=%d, content-length=%d", resp.StatusCode, resp.ContentLength))

	// 处理重定向（tls-client 设置了 WithNotFollowRedirects，手动跟随保持 TLS 指纹一致）
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := resp.Header.Get("Location")
		if location != "" {
			SysLog(fmt.Sprintf("Following redirect to: %s", location))
			resp.Body.Close()

			redirectURL, err := url.Parse(location)
			if err != nil {
				return nil, fmt.Errorf("failed to parse redirect URL: %w", err)
			}
			baseURL, _ := url.Parse(sourceURL)
			finalURL := baseURL.ResolveReference(redirectURL).String()

			req2, err := fhttp.NewRequest("GET", finalURL, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create redirect request: %w", err)
			}
			req2.Header = req.Header

			resp, err = client.Do(req2)
			if err != nil {
				return nil, fmt.Errorf("redirect request failed: %w", err)
			}
			defer resp.Body.Close()

			SysLog(fmt.Sprintf("Redirect response: status=%d, content-length=%d", resp.StatusCode, resp.ContentLength))
		}
	}

	if resp.StatusCode == 403 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		SysError(fmt.Sprintf("403 Forbidden, response body (first 500 bytes): %s", string(errBody[:min(500, len(errBody))])))
		return nil, fmt.Errorf("HTTP 403 Forbidden (Cloudflare blocked)")
	}

	if resp.StatusCode != 200 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		SysError(fmt.Sprintf("HTTP error: status=%d, body=%s", resp.StatusCode, string(errBody)))
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	SysLog(fmt.Sprintf("Downloaded %d bytes (%.2f MB)", len(data), float64(len(data))/(1024*1024)))

	// 检查是否下载到 HTML 错误页面
	if len(data) < 100000 && bytes.Contains(data, []byte("<html")) {
		SysError(fmt.Sprintf("Downloaded HTML instead of video (first 500 bytes): %s", string(data[:min(500, len(data))])))
		return nil, fmt.Errorf("downloaded HTML error page instead of video")
	}

	logVideoFileHeader(data)
	SysLog(fmt.Sprintf("=== tls-client download completed: %.2f MB ===", float64(len(data))/(1024*1024)))
	return data, nil
}

// logVideoFileHeader 记录视频文件头信息用于调试
func logVideoFileHeader(data []byte) {
	if len(data) > 12 {
		header := fmt.Sprintf("%x", data[:12])
		SysLog(fmt.Sprintf("File header (first 12 bytes): %s", header))
		if bytes.Contains(data[:min(20, len(data))], []byte("ftyp")) {
			SysLog("Confirmed MP4 video file format")
		} else {
			SysLog("Warning: File header doesn't match expected MP4 format")
		}
	}
}

// DownloadVideoWithChromedp 保留兼容性接口，内部使用 tls-client 实现
func DownloadVideoWithChromedp(sourceURL string) ([]byte, error) {
	return downloadWithTLSClient(context.Background(), sourceURL, "")
}
