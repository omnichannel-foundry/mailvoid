package web

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gnrfan/mailvoid/internal/mailbox"
	"github.com/gnrfan/mailvoid/internal/storage"
	"github.com/gofiber/fiber/v2"
	"golang.org/x/net/webdav"
)

func (s *Server) requireWebDAVAuth(c *fiber.Ctx) error {
	if !s.webdavEnabled.Load() {
		return fiber.ErrNotFound
	}
	header := strings.TrimSpace(c.Get(fiber.HeaderAuthorization))
	const prefix = "Basic "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return webDAVChallenge(c)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(header[len(prefix):]))
	if err != nil {
		return webDAVChallenge(c)
	}
	username, password, ok := strings.Cut(string(decoded), ":")
	if !ok {
		return webDAVChallenge(c)
	}
	if s.cfg.WebDAVPassword != "" &&
		subtle.ConstantTimeCompare([]byte(username), []byte(s.cfg.WebDAVUser)) == 1 &&
		subtle.ConstantTimeCompare([]byte(password), []byte(s.cfg.WebDAVPassword)) == 1 {
		c.Locals("user", "webdav")
		return c.Next()
	}
	if err := s.auth.VerifyWebDAV(c.UserContext(), username, password); err != nil {
		return webDAVChallenge(c)
	}
	c.Locals("user", "webdav:"+strings.ToLower(username))
	return c.Next()
}

func webDAVChallenge(c *fiber.Ctx) error {
	c.Set(fiber.HeaderWWWAuthenticate, `Basic realm="MailVoid WebDAV", charset="UTF-8"`)
	return c.Status(fiber.StatusUnauthorized).SendString("authentication required")
}

func webDAVReadOnly(c *fiber.Ctx) error {
	switch c.Method() {
	case fiber.MethodOptions, fiber.MethodGet, fiber.MethodHead, "PROPFIND":
		return c.Next()
	default:
		c.Set(fiber.HeaderAllow, "OPTIONS, PROPFIND, GET, HEAD")
		return c.SendStatus(fiber.StatusMethodNotAllowed)
	}
}

// davFS is a read-only virtual filesystem:
//
//	/dav/                              mailboxes
//	/dav/<mailbox@domain>/             messages in that mailbox
//	/dav/<mailbox@domain>/<id>.eml     raw message
type davFS struct {
	store     storage.Store
	mailboxes *mailbox.Service
}

func (f *davFS) Mkdir(context.Context, string, os.FileMode) error {
	return os.ErrPermission
}

func (f *davFS) RemoveAll(context.Context, string) error {
	return os.ErrPermission
}

func (f *davFS) Rename(context.Context, string, string) error {
	return os.ErrPermission
}

func (f *davFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	file, err := f.OpenFile(ctx, name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return file.Stat()
}

func (f *davFS) OpenFile(ctx context.Context, name string, flag int, _ os.FileMode) (webdav.File, error) {
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC|os.O_APPEND) != 0 {
		return nil, os.ErrPermission
	}

	clean := path.Clean("/" + strings.TrimPrefix(name, "/"))
	trimmed := strings.Trim(clean, "/")
	if trimmed == "" {
		entries, err := f.rootEntries(ctx)
		if err != nil {
			return nil, err
		}
		return newDAVFile(davFileInfo{name: "/", dir: true}, nil, entries), nil
	}

	segments := strings.Split(trimmed, "/")
	if len(segments) == 1 {
		entries, err := f.mailboxEntries(ctx, segments[0])
		if err != nil {
			return nil, err
		}
		return newDAVFile(davFileInfo{name: segments[0], dir: true, modTime: latestModTime(entries)}, nil, entries), nil
	}
	if len(segments) != 2 {
		return nil, os.ErrNotExist
	}

	mailboxName, domain, err := mailbox.ParseAddress(segments[0])
	if err != nil {
		return nil, os.ErrNotExist
	}
	filename := segments[1]
	if !strings.HasSuffix(strings.ToLower(filename), ".eml") {
		return nil, os.ErrNotExist
	}
	messageID := filename[:len(filename)-4]
	message, err := f.store.GetMessage(ctx, messageID)
	if err != nil || message.Mailbox != mailboxName || message.Domain != domain {
		return nil, os.ErrNotExist
	}
	return newDAVFile(davFileInfo{
		name:    filename,
		size:    int64(len(message.Raw)),
		modTime: message.ReceivedAt,
	}, bytes.NewReader([]byte(message.Raw)), nil), nil
}

func (f *davFS) rootEntries(ctx context.Context) ([]os.FileInfo, error) {
	summaries, err := f.mailboxes.All(ctx, "", time.Time{})
	if err != nil {
		return nil, err
	}
	entries := make([]os.FileInfo, 0, len(summaries))
	for _, summary := range summaries {
		var modTime time.Time
		if summary.LatestReceivedAt != nil {
			modTime = *summary.LatestReceivedAt
		}
		entries = append(entries, davFileInfo{
			name:    summary.Address,
			dir:     true,
			modTime: modTime,
		})
	}
	return entries, nil
}

func (f *davFS) mailboxEntries(ctx context.Context, address string) ([]os.FileInfo, error) {
	messages, err := f.mailboxes.AllMessages(ctx, address, mailbox.MessageOptions{})
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, os.ErrNotExist
	}
	entries := make([]os.FileInfo, 0, len(messages))
	for _, message := range messages {
		entries = append(entries, davFileInfo{
			name:    message.ID + ".eml",
			size:    int64(len(message.Raw)),
			modTime: message.ReceivedAt,
		})
	}
	return entries, nil
}

func latestModTime(entries []os.FileInfo) time.Time {
	var latest time.Time
	for _, entry := range entries {
		if entry.ModTime().After(latest) {
			latest = entry.ModTime()
		}
	}
	return latest
}

type davFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	dir     bool
}

func (i davFileInfo) Name() string { return i.name }
func (i davFileInfo) Size() int64  { return i.size }
func (i davFileInfo) Mode() os.FileMode {
	if i.dir {
		return os.ModeDir | 0o755
	}
	return i.mode | 0o644
}
func (i davFileInfo) ModTime() time.Time { return i.modTime }
func (i davFileInfo) IsDir() bool        { return i.dir }
func (i davFileInfo) Sys() any           { return nil }

type davFile struct {
	info    os.FileInfo
	reader  *bytes.Reader
	entries []os.FileInfo
}

func newDAVFile(info os.FileInfo, reader *bytes.Reader, entries []os.FileInfo) *davFile {
	return &davFile{info: info, reader: reader, entries: entries}
}

func (f *davFile) Close() error { return nil }

func (f *davFile) Read(p []byte) (int, error) {
	if f.reader == nil {
		return 0, io.EOF
	}
	return f.reader.Read(p)
}

func (f *davFile) Seek(offset int64, whence int) (int64, error) {
	if f.reader == nil {
		return 0, os.ErrInvalid
	}
	return f.reader.Seek(offset, whence)
}

func (f *davFile) Readdir(count int) ([]os.FileInfo, error) {
	if !f.info.IsDir() {
		return nil, os.ErrInvalid
	}
	if count <= 0 {
		out := f.entries
		f.entries = nil
		return out, nil
	}
	if len(f.entries) == 0 {
		return nil, io.EOF
	}
	n := count
	if n > len(f.entries) {
		n = len(f.entries)
	}
	out := f.entries[:n]
	f.entries = f.entries[n:]
	return out, nil
}

func (f *davFile) Stat() (os.FileInfo, error) { return f.info, nil }

func (f *davFile) Write([]byte) (int, error) { return 0, os.ErrPermission }
