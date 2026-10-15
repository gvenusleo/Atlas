// Command atlas 启动 Atlas 桌面应用。
package main

import (
	"log"

	"github.com/gvenusleo/atlas/internal/gui"
)

func main() {
	if err := gui.Run(); err != nil {
		log.Fatal(err)
	}
}
