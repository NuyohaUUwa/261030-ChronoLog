//go:build !dev

package assets

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web_dist/*
var embeddedFiles embed.FS

// GetFileSystem 返回生产环境下内嵌的前端静态资源文件系统
func GetFileSystem() (http.FileSystem, bool) {
	fsys, err := fs.Sub(embeddedFiles, "web_dist")
	if err != nil {
		return nil, false
	}
	return http.FS(fsys), true
}
