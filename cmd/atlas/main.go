// Command atlas 启动 Atlas 桌面应用：打开数据库，建核心，桌面界面作为它的客户端。
package main

import (
	"context"
	"log"
	"path/filepath"
	"time"

	"github.com/gvenusleo/atlas/internal/config"
	"github.com/gvenusleo/atlas/internal/core"
	"github.com/gvenusleo/atlas/internal/gui"
	"github.com/gvenusleo/atlas/internal/store"
	"github.com/gvenusleo/atlas/internal/tools"
)

func main() {
	dir, err := config.Dir()
	if err != nil {
		log.Fatal(err)
	}
	db, err := store.Open(context.Background(), filepath.Join(dir, "atlas.db"))
	if err != nil {
		log.Fatal(err)
	}
	c := core.New(db)
	go func() {
		if err := tools.PruneShellLogs(time.Now()); err != nil {
			log.Printf("清理命令日志失败: %v", err)
		}
	}()
	if err := gui.Run(c.Connect, c.Close); err != nil {
		log.Fatal(err)
	}
}
