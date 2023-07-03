package tools

import (
	"fmt"
	"os"
)

var ENV_DEBUG bool

// dd 命令
func DD(v ...any) {
	fmt.Println(v)
	os.Exit(0)
}
