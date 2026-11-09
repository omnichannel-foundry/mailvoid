package events

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// FileSink appends one JSON object per line to a log file.
type FileSink struct {
	mu     sync.Mutex
	file   *os.File
	closed bool
}

// NewFileSink opens (or creates) path for append-only JSONL output.
func NewFileSink(path string) (*FileSink, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &FileSink{file: f}, nil
}

// Write appends and flushes one JSON line.
func (s *FileSink) Write(ev Event) error {
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return os.ErrClosed
	}
	_, err = s.file.Write(line)
	return err
}

// Close closes the underlying file.
func (s *FileSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.file.Close()
}
