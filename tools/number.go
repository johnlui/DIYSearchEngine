package tools

import (
  "golang.org/x/text/language"
  "golang.org/x/text/message"
)

func AddDouhao(v int) string {
  p := message.NewPrinter(language.English)
  return p.Sprintf("%d", v)
}
