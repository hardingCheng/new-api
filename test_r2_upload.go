package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/joho/godotenv"
)

func main() {
	fmt.Println("=== Cloudflare R2 Upload Test ===")
	
	// 1. 加载环境变量
	err := godotenv.Load(".env")
	if err != nil {
		log.Printf("Warning: No .env file found: %v\n", err)
	}
	
	// 2. 初始化环境变量
	common.InitEnv()
	
	// 3. 检查 R2 配置
	fmt.Printf("\nR2 Configuration:\n")
	fmt.Printf("  Enabled: %v\n", common.R2VideoUploadEnabled)
	fmt.Printf("  Expiry Days: %d\n", common.R2VideoExpiryDays)
	
	if !common.R2VideoUploadEnabled {
		log.Fatal("R2 upload is not enabled. Please set R2_VIDEO_UPLOAD_ENABLED=true in .env")
	}
	
	uploader := common.GetR2Uploader()
	if uploader == nil {
		log.Fatal("R2 uploader is not initialized")
	}
	
	fmt.Println("\n✓ R2 uploader initialized successfully")
	
	// 4. 测试上传一个示例视频
	// 使用一个公开的测试视频 URL
	testVideoURL := "https://videos.openai.com/az/files/00000000-7f98-7284-a1a5-dbe5c2e28029%2Fraw?se=2026-03-14T00%3A00%3A00Z&sp=r&sv=2026-02-06&sr=b&skoid=3d249c53-07fa-4ba4-9b65-0bf8eb4ea46a&sktid=a48cca56-e6da-484e-a814-9c849652bcb3&skt=2026-03-08T03%3A20%3A40Z&ske=2026-03-15T03%3A25%3A40Z&sks=b&skv=2026-02-06&sig=XrZChcpPS4L3JjRDKn3XohPZ8W3pNlAHqYv4I63/31I%3D&ac=oaisdsorprcentralus"
	testTaskID := fmt.Sprintf("test_%d", time.Now().Unix())
	
	fmt.Printf("\nTest Parameters:\n")
	fmt.Printf("  Source URL: %s\n", testVideoURL)
	fmt.Printf("  Task ID: %s\n", testTaskID)
	
	// 生成对象键
	objectKey := common.GenerateR2ObjectKey(testTaskID, testVideoURL)
	fmt.Printf("  Object Key: %s\n", objectKey)
	
	// 5. 执行上传
	fmt.Println("\nStarting upload...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	
	startTime := time.Now()
	r2URL, err := uploader.DownloadAndUpload(ctx, testVideoURL, objectKey)
	duration := time.Since(startTime)
	
	if err != nil {
		log.Fatalf("Upload failed: %v", err)
	}
	
	fmt.Printf("\n✓ Upload successful!\n")
	fmt.Printf("  Duration: %v\n", duration)
	fmt.Printf("  R2 URL: %s\n", r2URL)
	
	// 6. 测试清理功能（可选）
	fmt.Println("\n=== Testing Cleanup Function ===")
	fmt.Println("Note: This will only delete files older than the expiry period")
	
	err = common.CleanupExpiredR2Videos()
	if err != nil {
		log.Printf("Cleanup test failed: %v", err)
	} else {
		fmt.Println("✓ Cleanup test completed")
	}
	
	fmt.Println("\n=== Test Complete ===")
	fmt.Println("\nNext steps:")
	fmt.Println("1. Check if the video is accessible at the R2 URL above")
	fmt.Println("2. Verify the file exists in your R2 bucket")
	fmt.Println("3. Start your application with: go run main.go")
	fmt.Println("\nNote: R2 public access must be enabled for the URL to work")
	fmt.Println("      Go to Cloudflare Dashboard → R2 → Your Bucket → Settings → Public Access")
}
