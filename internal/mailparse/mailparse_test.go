package mailparse_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/gnrfan/mailvoid/internal/mailparse"
)

func TestSimplePlainText(t *testing.T) {
	raw := "From: sender@example.org\r\n" +
		"To: inbox@example.test\r\n" +
		"Subject: Hello there\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"Hello, world!\r\n"

	parsed, err := mailparse.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Subject != "Hello there" {
		t.Fatalf("subject = %q", parsed.Subject)
	}
	if strings.TrimSpace(parsed.Text) != "Hello, world!" {
		t.Fatalf("text = %q", parsed.Text)
	}
	if parsed.HTML != "" {
		t.Fatalf("html = %q", parsed.HTML)
	}
	if !strings.Contains(parsed.RawHeaders, "Subject: Hello there") {
		t.Fatalf("raw headers = %q", parsed.RawHeaders)
	}
	if got := parsed.Header["Subject"]; len(got) != 1 || got[0] != "Hello there" {
		t.Fatalf("headers = %+v", parsed.Header)
	}
}

func TestEncodedSubject(t *testing.T) {
	raw := "Subject: =?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte("Héllo wörld")) + "?=\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\nbody"
	parsed, err := mailparse.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Subject != "Héllo wörld" {
		t.Fatalf("subject = %q", parsed.Subject)
	}
}

func TestMultipartAlternative(t *testing.T) {
	boundary := "bnd42"
	text := base64.StdEncoding.EncodeToString([]byte("plain body"))
	html := base64.StdEncoding.EncodeToString([]byte("<p>html body</p>"))
	raw := "Subject: Multi\r\n" +
		"Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n" +
		"\r\n" +
		"--" + boundary + "\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" + text + "\r\n" +
		"--" + boundary + "\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" + html + "\r\n" +
		"--" + boundary + "--\r\n"

	parsed, err := mailparse.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Text != "plain body" {
		t.Fatalf("text = %q", parsed.Text)
	}
	if parsed.HTML != "<p>html body</p>" {
		t.Fatalf("html = %q", parsed.HTML)
	}
}

func TestQuotedPrintableAndLatin1(t *testing.T) {
	raw := "Subject: QP\r\n" +
		"Content-Type: text/plain; charset=iso-8859-1\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n" +
		"\r\n" +
		"caf=E9 and line=\r\nbreak"
	parsed, err := mailparse.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Text != "café and linebreak" {
		t.Fatalf("text = %q", parsed.Text)
	}
}

func TestHTMLOnly(t *testing.T) {
	raw := "Subject: HTML\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<h1>Hi</h1>"
	parsed, err := mailparse.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.HTML != "<h1>Hi</h1>" || parsed.Text != "" {
		t.Fatalf("parsed = %+v", parsed)
	}
}

func TestMalformedMessage(t *testing.T) {
	if _, err := mailparse.Parse([]byte("not a message")); err == nil {
		t.Fatal("expected error")
	}
}

func TestAttachmentIgnored(t *testing.T) {
	boundary := "mix"
	raw := "Subject: Mixed\r\n" +
		"Content-Type: multipart/mixed; boundary=" + boundary + "\r\n" +
		"\r\n" +
		"--" + boundary + "\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"main body\r\n" +
		"--" + boundary + "\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Disposition: attachment; filename=\"note.txt\"\r\n" +
		"\r\n" +
		"attachment text\r\n" +
		"--" + boundary + "--\r\n"

	parsed, err := mailparse.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(parsed.Text, "main body") {
		t.Fatalf("text = %q", parsed.Text)
	}
	if strings.Contains(parsed.Text, "attachment text") {
		t.Fatalf("attachment leaked into body: %q", parsed.Text)
	}
}

func TestNestedMultipart(t *testing.T) {
	outer, inner := "outer", "inner"
	raw := "Subject: nested\r\n" +
		"Content-Type: multipart/mixed; boundary=" + outer + "\r\n\r\n" +
		"--" + outer + "\r\n" +
		"Content-Type: multipart/alternative; boundary=" + inner + "\r\n\r\n" +
		"--" + inner + "\r\nContent-Type: text/plain\r\n\r\nnested text\r\n" +
		"--" + inner + "\r\nContent-Type: text/html\r\n\r\n<p>nested html</p>\r\n" +
		"--" + inner + "--\r\n" +
		"--" + outer + "--\r\n"
	parsed, err := mailparse.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Text != "nested text" && strings.TrimSpace(parsed.Text) != "nested text" {
		t.Fatalf("text = %q", parsed.Text)
	}
	if !strings.Contains(parsed.HTML, "nested html") {
		t.Fatalf("html = %q", parsed.HTML)
	}
}

func TestFirstTextPartWins(t *testing.T) {
	boundary := "b"
	raw := "Content-Type: multipart/mixed; boundary=" + boundary + "\r\n\r\n" +
		"--" + boundary + "\r\nContent-Type: text/plain\r\n\r\nfirst\r\n" +
		"--" + boundary + "\r\nContent-Type: text/plain\r\n\r\nsecond\r\n" +
		"--" + boundary + "--\r\n"
	parsed, err := mailparse.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(parsed.Text) != "first" {
		t.Fatalf("text = %q", parsed.Text)
	}
}

func TestNoContentTypeAndEmptyBody(t *testing.T) {
	parsed, err := mailparse.Parse([]byte("Subject: none\r\n\r\nplain body"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Text != "plain body" || parsed.HTML != "" {
		t.Fatalf("parsed = %+v", parsed)
	}

	parsed, err = mailparse.Parse([]byte("Subject: empty\r\n\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Text != "" || parsed.HTML != "" {
		t.Fatalf("empty = %+v", parsed)
	}
}

func TestWindows1252Body(t *testing.T) {
	raw := []byte("Subject: cp1252\r\nContent-Type: text/plain; charset=windows-1252\r\n\r\n")
	raw = append(raw, 0x93, 'q', 0x94)
	parsed, err := mailparse.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Text != "“q”" {
		t.Fatalf("text = %q", parsed.Text)
	}
}

func TestUnknownCharsetPassesThrough(t *testing.T) {
	raw := []byte("Subject: unknown\r\nContent-Type: text/plain; charset=x-unknown\r\n\r\n")
	raw = append(raw, 0xff)
	parsed, err := mailparse.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Text != string([]byte{0xff}) {
		t.Fatalf("text = %q", parsed.Text)
	}
}

func TestEncodedSubjectISO8859(t *testing.T) {
	raw := "Subject: =?iso-8859-1?Q?caf=E9?=\r\nContent-Type: text/plain\r\n\r\nbody"
	parsed, err := mailparse.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Subject != "café" {
		t.Fatalf("subject = %q", parsed.Subject)
	}
}

func TestInlineAttachmentIsIncluded(t *testing.T) {
	raw := "Content-Type: text/plain\r\nContent-Disposition: inline\r\n\r\ninline text"
	parsed, err := mailparse.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Text != "inline text" {
		t.Fatalf("text = %q", parsed.Text)
	}
}

func TestMessageRFC822Ignored(t *testing.T) {
	boundary := "b"
	raw := "Content-Type: multipart/mixed; boundary=" + boundary + "\r\n\r\n" +
		"--" + boundary + "\r\nContent-Type: text/plain\r\n\r\nouter\r\n" +
		"--" + boundary + "\r\nContent-Type: message/rfc822\r\n\r\nSubject: inner\r\n\r\ninner\r\n" +
		"--" + boundary + "--\r\n"
	parsed, err := mailparse.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(parsed.Text, "outer") || strings.Contains(parsed.Text, "inner") {
		t.Fatalf("text = %q", parsed.Text)
	}
}

func TestMalformedBase64FallsBack(t *testing.T) {
	raw := "Subject: bad b64\r\nContent-Type: text/plain\r\nContent-Transfer-Encoding: base64\r\n\r\n!!!!"
	parsed, err := mailparse.Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Text != "!!!!" {
		t.Fatalf("text = %q", parsed.Text)
	}
}
