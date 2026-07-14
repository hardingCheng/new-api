package chatdump

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileInfo 列表项摘要。
type FileInfo struct {
	Date    string `json:"date"`
	Name    string `json:"name"`
	SizeKb  int64  `json:"size_kb"`
	ModTime int64  `json:"mod_time"`
}

// ListFilter 过滤条件。
type ListFilter struct {
	Date  string // YYYY-MM-DD，留空=全部
	Model string // 模型关键字，子串匹配（小写）
}

// DumpRoot 返回当前抓取目录（已确保初始化）。
func DumpRoot() string {
	initConfig()
	return dumpDir
}

// IsEnabled 返回抓取功能是否启用。
func IsEnabled() bool {
	initConfig()
	return dumpEnabled
}

// ListDates 列出存在的日期目录，新到旧。
func ListDates() ([]string, error) {
	initConfig()
	entries, err := os.ReadDir(dumpDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	dates := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			dates = append(dates, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))
	return dates, nil
}

// fileRef 是一个文件的轻量引用，只含日期目录与文件名，不含 stat 信息。
type fileRef struct {
	date string
	name string
}

// collectRefs 按「新到旧」收集所有匹配的文件名引用，全程只读目录、不 stat 文件。
// 几十万文件下，stat（lstat）才是开销大头，这里把它推迟到真正要返回的那一页。
func collectRefs(filter ListFilter) ([]fileRef, error) {
	initConfig()
	dates := []string{}
	if filter.Date != "" {
		dates = []string{filter.Date}
	} else {
		ds, err := ListDates()
		if err != nil {
			return nil, err
		}
		dates = ds
	}
	modelKey := strings.ToLower(filter.Model)
	refs := make([]fileRef, 0, 1024)
	for _, d := range dates {
		dir := filepath.Join(dumpDir, d)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			if modelKey != "" && !strings.Contains(strings.ToLower(e.Name()), modelKey) {
				continue
			}
			names = append(names, e.Name())
		}
		// 文件名前缀就是 HHMMSSmmm，按字符串倒序天然就是时间新到旧
		sort.Sort(sort.Reverse(sort.StringSlice(names)))
		for _, n := range names {
			refs = append(refs, fileRef{date: d, name: n})
		}
	}
	return refs, nil
}

// statRefs 对给定引用逐个 stat，构建列表项；stat 失败的条目跳过。
func statRefs(refs []fileRef) []FileInfo {
	out := make([]FileInfo, 0, len(refs))
	for _, ref := range refs {
		info, err := os.Stat(filepath.Join(dumpDir, ref.date, ref.name))
		if err != nil {
			continue
		}
		out = append(out, FileInfo{
			Date:    ref.date,
			Name:    ref.name,
			SizeKb:  (info.Size() + 1023) / 1024,
			ModTime: info.ModTime().Unix(),
		})
	}
	return out
}

// ListFilesPaged 分页列出文件，新到旧。limit <= 0 表示返回全部（从 offset 起）。
// 返回当前页 + 匹配总数，total 用于前端展示「已加载 X/总数」与判断是否还有更多。
func ListFilesPaged(filter ListFilter, offset, limit int) ([]FileInfo, int, error) {
	refs, err := collectRefs(filter)
	if err != nil {
		return nil, 0, err
	}
	total := len(refs)
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return statRefs(refs[offset:end]), total, nil
}

// ListFiles 列出全部匹配文件，新到旧（导出/打包等需要全量时使用）。
func ListFiles(filter ListFilter) ([]FileInfo, error) {
	files, _, err := ListFilesPaged(filter, 0, 0)
	return files, err
}

// ReadFile 读取一个 dump 文件原始字节。
func ReadFile(date, name string) ([]byte, error) {
	initConfig()
	if err := validatePathPart(date); err != nil {
		return nil, err
	}
	if err := validatePathPart(name); err != nil {
		return nil, err
	}
	if !strings.HasSuffix(name, ".json") {
		return nil, errors.New("invalid file name")
	}
	path := filepath.Join(dumpDir, date, name)
	return os.ReadFile(path)
}

// DeleteFile 删除一个 dump 文件。
func DeleteFile(date, name string) error {
	initConfig()
	if err := validatePathPart(date); err != nil {
		return err
	}
	if err := validatePathPart(name); err != nil {
		return err
	}
	if !strings.HasSuffix(name, ".json") {
		return errors.New("invalid file name")
	}
	return os.Remove(filepath.Join(dumpDir, date, name))
}

// WriteZip 将匹配文件打包写入 w。
func WriteZip(filter ListFilter, w io.Writer) error {
	files, err := ListFiles(filter)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(w)
	defer zw.Close()
	for _, f := range files {
		path := filepath.Join(dumpDir, f.Date, f.Name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fw, err := zw.Create(f.Date + "/" + f.Name)
		if err != nil {
			return err
		}
		if _, err := fw.Write(data); err != nil {
			return err
		}
	}
	return nil
}

// validatePathPart 防止目录穿越。
func validatePathPart(p string) error {
	if p == "" {
		return errors.New("empty path part")
	}
	if strings.ContainsAny(p, "/\\") || strings.Contains(p, "..") {
		return errors.New("invalid path part")
	}
	return nil
}
