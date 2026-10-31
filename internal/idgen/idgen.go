package idgen

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

const randomBytes = 10

var validPrefix = regexp.MustCompile(`^[a-z][a-z0-9]{0,11}$`)

func New(prefix string) (string, error) {
	return generate(prefix, time.Now().UTC(), rand.Reader)
}

func generate(prefix string, now time.Time, source io.Reader) (string, error) {
	if !validPrefix.MatchString(prefix) {
		return "", fmt.Errorf("id prefix %q must match %s", prefix, validPrefix)
	}
	buffer := make([]byte, randomBytes)
	if _, err := io.ReadFull(source, buffer); err != nil {
		return "", fmt.Errorf("read id randomness: %w", err)
	}
	randomPart := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buffer))
	return fmt.Sprintf("%s_%013d_%s", prefix, now.UnixMilli(), randomPart), nil
}
