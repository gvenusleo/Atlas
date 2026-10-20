//go:build !darwin && !linux

package shellenv

import "context"

// load 在其他平台上什么也不取：Windows 上图形应用本来就拿到完整的用户环境。
func load(context.Context) ([]string, error) { return nil, nil }
