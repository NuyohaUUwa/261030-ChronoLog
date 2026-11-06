//go:build dev

package assets

import (
	"net/http"
)

// GetFileSystem 在开发环境下返回 nil, false，表示静态文件由 Vite 开发服务提供
func GetFileSystem() (http.FileSystem, bool) {
	return nil, false
}
