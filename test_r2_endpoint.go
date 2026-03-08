package main

import (
	"fmt"
	"os"

	"github.com/QuantumNous/new-api/common"
	"github.com/joho/godotenv"
)

func main() {
	fmt.Println("=== R2 Endpoint Configuration Test ===\n")
	
	// Load environment variables
	err := godotenv.Load(".env")
	if err != nil {
		fmt.Printf("Warning: No .env file found: %v\n", err)
	}
	
	// Initialize environment
	common.InitEnv()
	
	// Display configuration
	fmt.Printf("R2 Configuration:\n")
	fmt.Printf("  Enabled: %v\n", common.R2VideoUploadEnabled)
	fmt.Printf("  Account ID: %s\n", os.Getenv("R2_ACCOUNT_ID"))
	fmt.Printf("  Bucket: %s\n", os.Getenv("R2_BUCKET_NAME"))
	fmt.Printf("  Region: %s\n", os.Getenv("R2_REGION"))
	fmt.Printf("  Access Key ID: %s...\n", os.Getenv("R2_ACCESS_KEY_ID")[:10])
	
	uploader := common.GetR2Uploader()
	if uploader == nil {
		fmt.Println("\n❌ R2 uploader not initialized")
		return
	}
	
	fmt.Println("\n✓ R2 uploader initialized successfully")
	fmt.Println("\nExpected endpoint format:")
	fmt.Printf("  https://%s.r2.cloudflarestorage.com\n", os.Getenv("R2_ACCOUNT_ID"))
	fmt.Println("\nThe SDK should use path-style URLs:")
	fmt.Printf("  https://%s.r2.cloudflarestorage.com/%s/path/to/object\n", 
		os.Getenv("R2_ACCOUNT_ID"), 
		os.Getenv("R2_BUCKET_NAME"))
	fmt.Println("\nNOT virtual-hosted-style (which was causing the TLS error):")
	fmt.Printf("  https://<access-key>.r2.cloudflarestorage.com/...\n")
}
