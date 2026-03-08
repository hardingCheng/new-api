package common

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
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
	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
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

	// 对 videos.openai.com 使用 curl 下载（绕过 Cloudflare TLS 指纹检测）
	if strings.Contains(sourceURL, "videos.openai.com") {
		SysLog("Using curl for Cloudflare-protected URL")
		fileData, err = downloadWithCurl(ctx, sourceURL, proxyURL)
		if err != nil {
			return "", fmt.Errorf("failed to download with curl: %w", err)
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

	// 2. 检测 Content-Type
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

	// 6. 上传到 R2（带重试）
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

// newChromeTLSTransport 创建使用 Chrome TLS 指纹的 http.RoundTripper
// 用于绕过 Cloudflare 的 TLS 指纹检测
func newChromeTLSTransport(proxyURL *url.URL) http.RoundTripper {
	t := &http.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			rawConn, err := (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			host, _, _ := net.SplitHostPort(addr)
			tlsConn := utls.UClient(rawConn, &utls.Config{ServerName: host}, utls.HelloChrome_Auto)
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				rawConn.Close()
				return nil, err
			}
			return tlsConn, nil
		},
	}
	if proxyURL != nil {
		t.Proxy = http.ProxyURL(proxyURL)
	}
	// 启用 HTTP/2 支持
	if err := http2.ConfigureTransport(t); err != nil {
		SysError(fmt.Sprintf("Failed to configure HTTP/2: %v", err))
	}
	return t
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
