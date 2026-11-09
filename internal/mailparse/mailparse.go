// Package mailparse extracts usable metadata and bodies from raw MIME
// messages without external binaries or services.
package mailparse

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

// Parsed holds the decoded pieces MailVoid stores and displays.
type Parsed struct {
	Header     map[string][]string
	Subject    string
	Text       string
	HTML       string
	RawHeaders string
}

// Parse decodes a raw RFC 5322 message.
func Parse(raw []byte) (*Parsed, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parse message: %w", err)
	}

	out := &Parsed{
		Header:     cloneHeader(msg.Header),
		RawHeaders: rawHeaders(raw),
	}
	out.Subject = decodeHeader(msg.Header.Get("Subject"))
	out.Text, out.HTML = extractBodies(textproto.MIMEHeader(msg.Header), msg.Body)
	return out, nil
}

func cloneHeader(h mail.Header) map[string][]string {
	out := make(map[string][]string, len(h))
	for k, v := range h {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func rawHeaders(raw []byte) string {
	if i := bytes.Index(raw, []byte("\r\n\r\n")); i >= 0 {
		return string(raw[:i])
	}
	if i := bytes.Index(raw, []byte("\n\n")); i >= 0 {
		return string(raw[:i])
	}
	return string(raw)
}

func decodeHeader(value string) string {
	if value == "" {
		return ""
	}
	decoder := &mime.WordDecoder{CharsetReader: charsetReader}
	decoded, err := decoder.DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
}

func extractBodies(h textproto.MIMEHeader, body io.Reader) (textBody, htmlBody string) {
	data, err := io.ReadAll(body)
	if err != nil {
		return "", ""
	}
	walkPart(h, data, &textBody, &htmlBody)
	return textBody, htmlBody
}

func walkPart(h textproto.MIMEHeader, data []byte, textBody, htmlBody *string) {
	if isAttachment(h) {
		return
	}
	decoded, err := decodeTransfer(h.Get("Content-Transfer-Encoding"), data)
	if err != nil {
		decoded = data
	}

	mediaType, params := parseMediaType(h.Get("Content-Type"))
	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return
		}
		reader := multipart.NewReader(bytes.NewReader(decoded), boundary)
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			partData, err := io.ReadAll(part)
			if err != nil {
				continue
			}
			walkPart(part.Header, partData, textBody, htmlBody)
			_ = part.Close()
		}
		return
	}

	charset := params["charset"]
	switch mediaType {
	case "text/plain", "":
		if *textBody == "" {
			*textBody = decodeCharset(decoded, charset)
		}
	case "text/html":
		if *htmlBody == "" {
			*htmlBody = decodeCharset(decoded, charset)
		}
	}
}

func isAttachment(h textproto.MIMEHeader) bool {
	disposition := strings.ToLower(h.Get("Content-Disposition"))
	return strings.HasPrefix(disposition, "attachment")
}

func parseMediaType(value string) (string, map[string]string) {
	if value == "" {
		return "", map[string]string{}
	}
	mediaType, params, err := mime.ParseMediaType(value)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(value)), map[string]string{}
	}
	return strings.ToLower(mediaType), params
}

func decodeTransfer(encoding string, data []byte) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		clean := strings.Map(func(r rune) rune {
			switch r {
			case '\r', '\n', ' ', '\t':
				return -1
			}
			return r
		}, string(data))
		return base64.StdEncoding.DecodeString(clean)
	case "quoted-printable":
		return io.ReadAll(quotedprintable.NewReader(bytes.NewReader(data)))
	default:
		return data, nil
	}
}

func decodeCharset(data []byte, charset string) string {
	charset = strings.ToLower(strings.TrimSpace(charset))
	switch charset {
	case "", "utf-8", "utf8", "us-ascii", "ascii":
		return string(data)
	case "iso-8859-1", "latin1", "latin-1":
		decoded, err := charmap.ISO8859_1.NewDecoder().Bytes(data)
		if err != nil {
			return string(data)
		}
		return string(decoded)
	case "windows-1252", "cp1252":
		decoded, err := charmap.Windows1252.NewDecoder().Bytes(data)
		if err != nil {
			return string(data)
		}
		return string(decoded)
	default:
		return string(data)
	}
}

func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(charset)) {
	case "utf-8", "utf8", "us-ascii", "ascii":
		return input, nil
	case "iso-8859-1", "latin1", "latin-1":
		return charmap.ISO8859_1.NewDecoder().Reader(input), nil
	case "windows-1252", "cp1252":
		return charmap.Windows1252.NewDecoder().Reader(input), nil
	default:
		return input, nil
	}
}
