package tools

import (
	"fmt"
	"os"
)

var ENV_DEBUG bool
var exit = os.Exit

// dd 命令
func DD(v ...any) {
	fmt.Println(v...)
	exit(0)
}
