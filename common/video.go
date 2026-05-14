package common

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// GetVideoDuration 使用纯 Go 库获取视频文件的时长（秒）。
// 当前优先支持 MP4/MOV/M4V/WebM 容器；不支持的格式由调用方决定是否回退。
func GetVideoDuration(_ context.Context, f io.ReadSeeker, ext string) (duration float64, err error) {
	ext = strings.ToLower(strings.TrimSpace(ext))
	SysLog(fmt.Sprintf("GetVideoDuration: ext=%s", ext))

	switch ext {
	case ".mp4", ".mov", ".m4v":
		duration, err = getM4ADuration(f)
	case ".webm":
		duration, err = getWebMDuration(f)
	default:
		return 0, fmt.Errorf("unsupported video format: %s", ext)
	}

	SysLog(fmt.Sprintf("GetVideoDuration: duration=%f", duration))
	return duration, err
}
