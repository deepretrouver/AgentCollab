package idgen

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestGenerateProducesPrefixedTimeOrderedID(t *testing.T) {
	first, err := generate("task", time.UnixMilli(1_000), bytes.NewReader(bytes.Repeat([]byte{0x01}, randomBytes)))
	if err != nil {
		t.Fatal(err)
	}
	second, err := generate("task", time.UnixMilli(1_001), bytes.NewReader(bytes.Repeat([]byte{0x02}, randomBytes)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first, "task_") {
		t.Fatalf("id = %q, want task prefix", first)
	}
	if first >= second {
		t.Fatalf("ids are not time ordered: %q >= %q", first, second)
	}
	if first == second {
		t.Fatal("generated IDs must be unique")
	}
}

func TestGenerateRejectsInvalidPrefix(t *testing.T) {
	for _, prefix := range []string{"", "UPPER", "has space", "toolongprefixvalue"} {
		if _, err := generate(prefix, time.Now(), bytes.NewReader(make([]byte, randomBytes))); err == nil {
			t.Fatalf("generate(%q) error = nil", prefix)
		}
	}
}
