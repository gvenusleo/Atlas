// Command atlas 启动 Atlas 桌面应用：打开数据库，建核心，桌面界面作为它的客户端。
package main

import (
	"context"
	"log"
	"path/filepath"

	"github.com/gvenusleo/atlas/internal/config"
	"github.com/gvenusleo/atlas/internal/core"
	"github.com/gvenusleo/atlas/internal/gui"
	"github.com/gvenusleo/atlas/internal/store"
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
	if err := gui.Run(c.Connect, c.Close); err != nil {
		log.Fatal(err)
	}
}
