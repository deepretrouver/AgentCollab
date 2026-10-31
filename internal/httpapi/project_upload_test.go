package httpapi

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"testing"
)

func TestParseWorkspaceUploadStreamsLargeDirectoryWithoutMultipartPartLimit(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("source_name", "large-project"); err != nil {
		t.Fatal(err)
	}
	const fileCount = 1200
	for index := 0; index < fileCount; index++ {
		name := fmt.Sprintf("file-%04d.txt", index)
		part, err := writer.CreateFormFile("files", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(part, "x"); err != nil {
			t.Fatal(err)
		}
		if err := writer.WriteField("paths", "src/"+name); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest("POST", "/workspace", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	uploads, sourceName, cleanup, err := parseWorkspaceUpload(request)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		t.Fatal(err)
	}
	if sourceName != "large-project" || len(uploads) != fileCount {
		t.Fatalf("source=%q uploads=%d", sourceName, len(uploads))
	}
	reader, err := uploads[fileCount-1].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil || string(content) != "x" {
		t.Fatalf("content=%q err=%v", content, err)
	}
}
