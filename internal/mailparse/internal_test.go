package mailparse

import (
	"io"
	"strings"
	"testing"
)

func TestRawHeadersVariants(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"crlf", "Subject: x\r\n\r\nbody", "Subject: x"},
		{"lf", "Subject: x\n\nbody", "Subject: x"},
		{"no separator", "Subject: x", "Subject: x"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rawHeaders([]byte(tc.raw)); got != tc.want {
				t.Fatalf("rawHeaders = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecodeCharset(t *testing.T) {
	cases := []struct {
		name    string
		data    []byte
		charset string
		want    string
	}{
		{"utf8", []byte("héllo"), "utf-8", "héllo"},
		{"empty", []byte("hello"), "", "hello"},
		{"ascii", []byte("hello"), "us-ascii", "hello"},
		{"latin1", []byte{'c', 'a', 'f', 0xe9}, "iso-8859-1", "café"},
		{"cp1252", []byte{0x93, 'q', 0x94}, "windows-1252", "“q”"},
		{"unknown", []byte{0xff}, "x-unknown", string([]byte{0xff})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := decodeCharset(tc.data, tc.charset); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCharsetReader(t *testing.T) {
	for _, charset := range []string{"utf-8", "us-ascii", "iso-8859-1", "latin1", "windows-1252", "unknown"} {
		reader, err := charsetReader(charset, strings.NewReader("abc"))
		if err != nil {
			t.Fatalf("%s: %v", charset, err)
		}
		if _, err := io.ReadAll(reader); err != nil {
			t.Fatalf("%s read: %v", charset, err)
		}
	}
}

func TestParseMediaType(t *testing.T) {
	mediaType, params := parseMediaType("")
	if mediaType != "" || len(params) != 0 {
		t.Fatalf("empty = %q %+v", mediaType, params)
	}
	mediaType, params = parseMediaType("TEXT/PLAIN; charset=utf-8")
	if mediaType != "text/plain" || params["charset"] != "utf-8" {
		t.Fatalf("valid = %q %+v", mediaType, params)
	}
	mediaType, _ = parseMediaType("not a media type")
	if !strings.Contains(mediaType, "not a media type") {
		t.Fatalf("invalid = %q", mediaType)
	}
}

func TestIsAttachment(t *testing.T) {
	if !isAttachment(map[string][]string{"Content-Disposition": {"Attachment; filename=x"}}) {
		t.Fatal("expected attachment")
	}
	if isAttachment(map[string][]string{"Content-Disposition": {"inline"}}) {
		t.Fatal("inline should not be an attachment")
	}
}

func TestDecodeTransfer(t *testing.T) {
	encoded := "aGVs\nbG8="
	decoded, err := decodeTransfer("base64", []byte(encoded))
	if err != nil || string(decoded) != "hello" {
		t.Fatalf("base64 = %q err=%v", decoded, err)
	}
	decoded, err = decodeTransfer("quoted-printable", []byte("caf=E9"))
	if err != nil || string(decoded) != string([]byte{'c', 'a', 'f', 0xe9}) {
		t.Fatalf("qp = %q err=%v", decoded, err)
	}
	decoded, err = decodeTransfer("", []byte("plain"))
	if err != nil || string(decoded) != "plain" {
		t.Fatalf("plain = %q err=%v", decoded, err)
	}
	if _, err := decodeTransfer("base64", []byte("!!!!")); err == nil {
		t.Fatal("expected base64 error")
	}
}

func TestWalkPartSkipsMissingBoundary(t *testing.T) {
	var text, html string
	walkPart(map[string][]string{"Content-Type": {"multipart/alternative"}}, []byte("body"), &text, &html)
	if text != "" || html != "" {
		t.Fatalf("text=%q html=%q", text, html)
	}
}
