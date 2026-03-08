package main

import (
	"fmt"
	"log"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/joho/godotenv"
)

func main() {
	fmt.Println("=== R2 清理功能测试 ===\n")
	
	// 1. 加载环境变量
	err := godotenv.Load(".env")
	if err != nil {
		log.Printf("Warning: No .env file found: %v\n", err)
	}
	
	// 2. 初始化
	common.InitEnv()
	
	if !common.R2VideoUploadEnabled {
		log.Fatal("R2 upload is not enabled")
	}
	
	uploader := common.GetR2Uploader()
	if uploader == nil {
		log.Fatal("R2 uploader is not initialized")
	}
	
	fmt.Printf("配置信息:\n")
	fmt.Printf("  过期天数: %d 天\n", common.R2VideoExpiryDays)
	expiryDate := time.Now().Add(-time.Duration(common.R2VideoExpiryDays) * 24 * time.Hour)
	fmt.Printf("  过期日期: %s 之前的文件将被删除\n\n", expiryDate.Format("2006-01-02 15:04:05"))
	
	// 3. 列出所有视频文件
	fmt.Println("=== 列出所有视频文件 ===")
	videos, err := common.ListR2Videos()
	if err != nil {
		log.Fatalf("列出文件失败: %v", err)
	}
	
	if len(videos) == 0 {
		fmt.Println("没有找到任何视频文件")
	} else {
		fmt.Printf("找到 %d 个视频文件:\n\n", len(videos))
		
		expiredCount := 0
		for i, video := range videos {
			age := time.Since(video.LastModified)
			ageHours := int(age.Hours())
			ageDays := ageHours / 24
			
			isExpired := video.LastModified.Before(expiryDate)
			status := "✓ 保留"
			if isExpired {
				status = "✗ 将被删除"
				expiredCount++
			}
			
			fmt.Printf("%d. %s\n", i+1, status)
			fmt.Printf("   文件: %s\n", video.Key)
			fmt.Printf("   大小: %.2f MB\n", float64(video.Size)/(1024*1024))
			fmt.Printf("   上传时间: %s\n", video.LastModified.Format("2006-01-02 15:04:05"))
			fmt.Printf("   文件年龄: %d 天 %d 小时\n", ageDays, ageHours%24)
			fmt.Printf("   URL: %s\n\n", video.URL)
		}
		
		fmt.Printf("统计: 总共 %d 个文件, %d 个将被删除, %d 个保留\n\n", 
			len(videos), expiredCount, len(videos)-expiredCount)
	}
	
	// 4. 执行清理
	fmt.Println("=== 执行清理 ===")
	fmt.Print("确认要执行清理吗? (y/N): ")
	
	var confirm string
	fmt.Scanln(&confirm)
	
	if confirm != "y" && confirm != "Y" {
		fmt.Println("已取消清理")
		return
	}
	
	err = common.CleanupExpiredR2Videos()
	if err != nil {
		log.Fatalf("清理失败: %v", err)
	}
	
	fmt.Println("\n✓ 清理完成")
	
	// 5. 再次列出文件
	fmt.Println("\n=== 清理后的文件列表 ===")
	videos, err = common.ListR2Videos()
	if err != nil {
		log.Fatalf("列出文件失败: %v", err)
	}
	
	if len(videos) == 0 {
		fmt.Println("没有剩余文件")
	} else {
		fmt.Printf("剩余 %d 个文件\n", len(videos))
	}
	
	fmt.Println("\n提示:")
	fmt.Println("- 如果想测试删除所有文件，可以临时设置 R2_VIDEO_EXPIRY_DAYS=0")
	fmt.Println("- 生产环境建议设置合理的过期天数（如 7 或 30 天）")
}
