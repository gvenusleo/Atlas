// Package shellenv 向用户的登录 shell 取一份环境，给 shell 工具、MCP 服务器这类子进程用。
//
// 从 Dock 或 Finder 启动的应用只有系统给的最小环境，没有 shell 配置文件里设置的 PATH 等变量，
// 找不到 brew、mise、nvm 装的命令；从终端启动的应用则继承了终端的环境。
package shellenv

import (
	"context"
	"log"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// timeout 限制登录 shell 取环境的耗时；配置文件很重的 shell 可能超时，超时就用当前环境。
const timeout = 5 * time.Second

var (
	warmed atomic.Bool
	once   sync.Once
	done   = make(chan struct{}) // 取完环境（成败都算）后关闭
	extra  []string              // 登录 shell 的环境，done 关闭之后只读
)

// Warm 在后台开始向登录 shell 取环境，应用启动时调用一次。没有调用过时（比如测试），
// 不会去问 shell，Environ 就是当前进程的环境。
func Warm() {
	once.Do(func() {
		warmed.Store(true)
		go func() {
			defer close(done)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			var err error
			if extra, err = load(ctx); err != nil {
				log.Printf("向登录 shell 取环境失败，使用当前环境: %v", err)
			}
		}()
	})
}

// Environ 返回子进程该用的环境：当前进程的，加上登录 shell 的，同名的以后者为准。
// Warm 之后的第一次调用要等取环境结束，最多等 timeout 那么久。
func Environ() []string {
	if !warmed.Load() {
		return os.Environ()
	}
	<-done
	return overlay(os.Environ(), extra)
}

// overlay 把 extra 叠在 base 上：同名的替换，没有的追加。
func overlay(base, extra []string) []string {
	env := slices.Clone(base)
	index := make(map[string]int, len(env))
	for i, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		index[key] = i
	}
	for _, kv := range extra {
		key, _, _ := strings.Cut(kv, "=")
		if i, ok := index[key]; ok {
			env[i] = kv
		} else {
			index[key] = len(env)
			env = append(env, kv)
		}
	}
	return env
}
