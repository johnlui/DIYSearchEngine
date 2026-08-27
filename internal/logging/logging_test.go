package logging

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestLoggingPrefixesLevels(t *testing.T) {
	var buffer bytes.Buffer
	originalOutput := log.Writer()
	originalFlags := log.Flags()
	t.Cleanup(func() {
		log.SetOutput(originalOutput)
		log.SetFlags(originalFlags)
	})
	log.SetOutput(&buffer)
	log.SetFlags(0)

	Infof("event=%s", "start")
	Errorf("event=%s", "fail")

	output := buffer.String()
	for _, want := range []string{"level=info event=start", "level=error event=fail"} {
		if !strings.Contains(output, want) {
			t.Fatalf("log output missing %q: %q", want, output)
		}
	}
}
