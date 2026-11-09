// Package smtpserver implements the wildcard multi-port SMTP capture service.
package smtpserver

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"

	"github.com/emersion/go-smtp"
	"github.com/gnrfan/mailvoid/internal/config"
	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/policy"
	"github.com/gnrfan/mailvoid/internal/storage"
)

// Backend implements smtp.Backend for MailVoid.
type Backend struct {
	cfg    config.Config
	store  storage.Store
	engine *policy.Engine
	bus    *events.Bus
}

// NewBackend creates an SMTP backend.
func NewBackend(cfg config.Config, store storage.Store, engine *policy.Engine, bus *events.Bus) *Backend {
	return &Backend{cfg: cfg, store: store, engine: engine, bus: bus}
}

// NewSession starts a per-connection SMTP session.
func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	remoteIP := hostOnly(c.Conn().RemoteAddr())
	localPort := portOnly(c.Conn().LocalAddr())

	s := &session{
		backend:   b,
		remoteIP:  remoteIP,
		localPort: localPort,
	}
	b.bus.Emit(events.New(events.TypeSMTPConnected, events.SeverityInfo, map[string]any{
		"remote_ip":  remoteIP,
		"local_port": localPort,
		"hostname":   c.Hostname(),
	}))
	return s, nil
}

// Server manages one SMTP listener per configured port.
type Server struct {
	backend   *Backend
	servers   []*smtp.Server
	listeners []net.Listener
	addrs     []net.Addr
	mu        sync.Mutex
	closed    bool
}

// New creates a multi-port SMTP server.
func New(cfg config.Config, store storage.Store, engine *policy.Engine, bus *events.Bus) *Server {
	return &Server{backend: NewBackend(cfg, store, engine, bus)}
}

// Start binds every configured port and begins serving in the background.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg := s.backend.cfg
	for _, port := range cfg.SMTPPorts {
		addr := net.JoinHostPort(cfg.SMTPHost, strconv.Itoa(port))
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			s.closeLocked()
			return fmt.Errorf("listen SMTP %s: %w", addr, err)
		}

		server := smtp.NewServer(s.backend)
		server.Domain = "mailvoid.local"
		server.MaxMessageBytes = cfg.MaxMessageBytes
		server.ReadTimeout = cfg.ReadTimeout
		server.WriteTimeout = cfg.ReadTimeout
		server.ErrorLog = log.New(io.Discard, "", 0)

		s.listeners = append(s.listeners, listener)
		s.servers = append(s.servers, server)
		s.addrs = append(s.addrs, listener.Addr())

		go func(srv *smtp.Server, l net.Listener) {
			if err := srv.Serve(l); err != nil && !errors.Is(err, smtp.ErrServerClosed) {
				s.backend.bus.Emit(events.New(events.TypeSMTPError, events.SeverityError, map[string]any{
					"error":      err.Error(),
					"local_port": portOnly(l.Addr()),
				}))
			}
		}(server, listener)
	}
	return nil
}

// Addrs returns the bound listener addresses (useful when port 0 is used).
func (s *Server) Addrs() []net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]net.Addr, len(s.addrs))
	copy(out, s.addrs)
	return out
}

// Close stops all listeners and active connections.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeLocked()
}

func (s *Server) closeLocked() error {
	if s.closed {
		return nil
	}
	s.closed = true
	var firstErr error
	for _, server := range s.servers {
		if err := server.Close(); err != nil && !errors.Is(err, smtp.ErrServerClosed) && firstErr == nil {
			firstErr = err
		}
	}
	for _, listener := range s.listeners {
		_ = listener.Close()
	}
	return firstErr
}

func hostOnly(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}

func portOnly(addr net.Addr) int {
	if addr == nil {
		return 0
	}
	_, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(port)
	return n
}

// String implements fmt.Stringer for diagnostics.
func (s *Server) String() string {
	return fmt.Sprintf("smtpserver(%d listeners)", len(s.Addrs()))
}
