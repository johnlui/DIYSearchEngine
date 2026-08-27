package logging

import "log"

func Infof(format string, args ...any) {
	log.Printf("level=info "+format, args...)
}

func Errorf(format string, args ...any) {
	log.Printf("level=error "+format, args...)
}
