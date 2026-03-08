package common

import (
	"bytes"
	"context"
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

// S3Config S3 配置
type S3Config struct {
	Endpoint        string // S3 端点，例如：s3.cstcloud.cn
	Bucket          string // 桶名
	AccessKeyID     string // Access Key ID
	AccessKeySecret string // Access Key Secret
	Region          string // 区域，默认为 us-east-1
}

var (
	// S3VideoUploadEnabled 是否启用视频自动上传到 S3
	S3VideoUploadEnabled = false
	// S3VideoExpiryDays 视频文件过期时间（天），0 表示永不过期
	S3VideoExpiryDays = 0
	// globalS3Uploader 全局 S3 上传器
	globalS3Uploader *S3Uploader
)

// InitS3 初始化 S3 配置
func InitS3() {
	S3VideoUploadEnabled = GetEnvOrDefaultBool("S3_VIDEO_UPLOAD_ENABLED", false)
	S3VideoExpiryDays = GetEnvOrDefault("S3_VIDEO_EXPIRY_DAYS", 0)
	
	if !S3VideoUploadEnabled {
		return
	}

	config := S3Config{
		Endpoint:        os.Getenv("S3_ENDPOINT"),
		Bucket:          os.Getenv("S3_BUCKET"),
		AccessKeyID:     os.Getenv("S3_ACCESS_KEY_ID"),
		AccessKeySecret: os.Getenv("S3_ACCESS_KEY_SECRET"),
		Region:          GetEnvOrDefaultString("S3_REGION", "us-east-1"),
	}

	if config.Endpoint == "" || config.Bucket == "" || config.AccessKeyID == "" || config.AccessKeySecret == "" {
		SysError("S3 configuration is incomplete, video upload to S3 will be disabled")
		S3VideoUploadEnabled = false
		return
	}

	globalS3Uploader = NewS3Uploader(config)
	if S3VideoExpiryDays > 0 {
		SysLog(fmt.Sprintf("S3 uploader initialized: endpoint=%s, bucket=%s, expiry=%d days", config.Endpoint, config.Bucket, S3VideoExpiryDays))
	} else {
		SysLog(fmt.Sprintf("S3 uploader initialized: endpoint=%s, bucket=%s, no expiry", config.Endpoint, config.Bucket))
	}
}

// GetS3Uploader 获取全局 S3 上传器
func GetS3Uploader() *S3Uploader {
	return globalS3Uploader
}

// S3Uploader S3 上传器
type S3Uploader struct {
	client   *s3.Client
	bucket   string
	endpoint string
}

// NewS3Uploader 创建 S3 上传器
func NewS3Uploader(config S3Config) *S3Uploader {
	if config.Region == "" {
		config.Region = "us-east-1"
	}

	// 创建 AWS 配置
	cfg := aws.Config{
		Region: config.Region,
		Credentials: credentials.NewStaticCredentialsProvider(
			config.AccessKeyID,
			config.AccessKeySecret,
			"",
		),
	}

	// 创建 S3 客户端
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("https://" + config.Endpoint)
		o.UsePathStyle = false // 使用虚拟主机样式
	})

	return &S3Uploader{
		client:   client,
		bucket:   config.Bucket,
		endpoint: config.Endpoint,
	}
}

// DownloadAndUpload 从 URL 下载文件并上传到 S3
// 返回 S3 中的文件 URL
func (u *S3Uploader) DownloadAndUpload(ctx context.Context, sourceURL, objectKey string) (string, error) {
	// 1. 下载文件
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

	// 3. 检测 Content-Type
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(fileData)
	}

	// 4. 准备上传参数
	putInput := &s3.PutObjectInput{
		Bucket:      aws.String(u.bucket),
		Key:         aws.String(objectKey),
		Body:        bytes.NewReader(fileData),
		ContentType: aws.String(contentType),
	}

	// 5. 如果设置了过期时间，添加元数据和过期标记
	if S3VideoExpiryDays > 0 {
		expiryTime := time.Now().Add(time.Duration(S3VideoExpiryDays) * 24 * time.Hour)
		// 添加自定义元数据记录过期时间
		putInput.Metadata = map[string]string{
			"expiry-time": expiryTime.Format(time.RFC3339),
			"expiry-days": fmt.Sprintf("%d", S3VideoExpiryDays),
		}
		// 设置对象过期时间（如果 S3 服务支持）
		putInput.Expires = aws.Time(expiryTime)
	}

	// 6. 上传到 S3
	_, err = u.client.PutObject(ctx, putInput)
	if err != nil {
		return "", fmt.Errorf("failed to upload to S3: %w", err)
	}

	// 7. 构建文件 URL
	fileURL := fmt.Sprintf("https://%s.%s/%s", u.bucket, u.endpoint, objectKey)

	return fileURL, nil
}

// GenerateObjectKey 生成 S3 对象键
// 格式：videos/{date}/{taskID}/{filename}
func GenerateObjectKey(taskID, originalURL string) string {
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
func CleanupExpiredVideos() error {
	if !S3VideoUploadEnabled || S3VideoExpiryDays <= 0 {
		return nil
	}

	uploader := GetS3Uploader()
	if uploader == nil {
		return fmt.Errorf("S3 uploader not initialized")
	}

	ctx := context.Background()
	
	// 计算过期日期
	expiryDate := time.Now().Add(-time.Duration(S3VideoExpiryDays) * 24 * time.Hour)
	expiryDateStr := expiryDate.Format("2006-01-02")
	
	SysLog(fmt.Sprintf("Starting cleanup of videos older than %s", expiryDateStr))
	
	// 列出 videos/ 前缀下的所有对象
	listInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(uploader.bucket),
		Prefix: aws.String("videos/"),
	}
	
	paginator := s3.NewListObjectsV2Paginator(uploader.client, listInput)
	deletedCount := 0
	
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list objects: %w", err)
		}
		
		for _, obj := range page.Contents {
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
		SysLog(fmt.Sprintf("Cleanup completed: deleted %d expired videos", deletedCount))
	} else {
		SysLog("Cleanup completed: no expired videos found")
	}
	
	return nil
}
