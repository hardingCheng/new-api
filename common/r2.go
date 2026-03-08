package common

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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
	
	if !R2VideoUploadEnabled {
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
	// 1. 下载文件
	SysLog(fmt.Sprintf("Downloading video from: %s", sourceURL))
	resp, err := http.Get(sourceURL)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download file: status code %d", resp.StatusCode)
	}

	// 2. 读取文件内容
	fileData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read file data: %w", err)
	}
	
	fileSize := len(fileData)
	SysLog(fmt.Sprintf("Downloaded %d bytes, uploading to R2...", fileSize))

	// 3. 检测 Content-Type
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(fileData)
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
	// 从原始 URL 中提取文件扩展名
	ext := filepath.Ext(originalURL)
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

	// 如果 R2VideoExpiryDays < 0，表示不启用清理
	// 如果 R2VideoExpiryDays = 0，表示删除所有文件
	// 如果 R2VideoExpiryDays > 0，表示删除指定天数之前的文件
	if R2VideoExpiryDays < 0 {
		return nil
	}

	uploader := GetR2Uploader()
	if uploader == nil {
		return fmt.Errorf("R2 uploader not initialized")
	}

	ctx := context.Background()
	
	// 计算过期日期
	var expiryDate time.Time
	if R2VideoExpiryDays == 0 {
		// 删除所有文件：使用未来的日期
		expiryDate = time.Now().Add(24 * time.Hour)
		SysLog("Starting cleanup of ALL R2 videos (expiry days = 0)")
	} else {
		expiryDate = time.Now().Add(-time.Duration(R2VideoExpiryDays) * 24 * time.Hour)
		SysLog(fmt.Sprintf("Starting cleanup of R2 videos older than %s", expiryDate.Format("2006-01-02")))
	}
	
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
