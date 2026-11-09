// Package smtpserver implements the wildcard multi-port SMTP capture service.
package smtpserver

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/emersion/go-smtp"
	"github.com/gnrfan/mailvoid/internal/capture"
	"github.com/gnrfan/mailvoid/internal/config"
	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/policy"
	"github.com/gnrfan/mailvoid/internal/storage"
)

// Backend implements smtp.Backend for MailVoid.
type Backend struct {
	cfg     config.Config
	service config.SMTPService
	store   storage.Store
	engine  *policy.Engine
	bus     *events.Bus
	logger  *log.Logger
	capture *capture.Controller
}

// NewBackend creates an SMTP backend.
func NewBackend(cfg config.Config, store storage.Store, engine *policy.Engine, bus *events.Bus) *Backend {
	return &Backend{cfg: cfg, store: store, engine: engine, bus: bus}
}

func (b *Backend) logf(format string, args ...any) {
	if b.logger != nil {
		b.logger.Printf(format, args...)
	}
}

func (b *Backend) captureEnabled() bool {
	return b.capture == nil || b.capture.Enabled()
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
	b.logf("connect remote=%s local_port=%d hostname=%q", remoteIP, localPort, c.Hostname())
	b.bus.Emit(events.New(events.TypeSMTPConnected, events.SeverityInfo, map[string]any{
		"remote_ip":  remoteIP,
		"local_port": localPort,
		"service":    b.service.LogName(),
		"aliases":    b.service.Aliases,
		"hostname":   c.Hostname(),
	}))
	return s, nil
}

type service struct {
	cfg      config.SMTPService
	backend  *Backend
	server   *smtp.Server
	listener net.Listener
	logFile  *os.File
}

// Server manages one SMTP listener per configured service.
type Server struct {
	cfg      config.Config
	store    storage.Store
	engine   *policy.Engine
	bus      *events.Bus
	capture  *capture.Controller
	services []*service
	mu       sync.Mutex
	closed   bool
}

// New creates a multi-service SMTP server.
func New(cfg config.Config, store storage.Store, engine *policy.Engine, bus *events.Bus) *Server {
	return &Server{cfg: cfg, store: store, engine: engine, bus: bus, capture: capture.New(true)}
}

// SetCaptureController shares the runtime capture switch with every listener.
func (s *Server) SetCaptureController(controller *capture.Controller) {
	s.capture = controller
}

// Start binds every configured service and begins serving in the background.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errors.New("SMTP server is closed")
	}

	services := s.cfg.EffectiveSMTPServices()
	if len(services) == 0 {
		return errors.New("no SMTP services configured")
	}

	for _, svcCfg := range services {
		addr := svcCfg.Address()
		logger, logFile, err := openServiceLog(s.cfg, svcCfg)
		if err != nil {
			s.closeLocked()
			return fmt.Errorf("open log for SMTP service %s: %w", svcCfg.LogName(), err)
		}

		backend := &Backend{
			cfg:     s.cfg,
			service: svcCfg,
			store:   s.store,
			engine:  s.engine,
			bus:     s.bus,
			logger:  logger,
			capture: s.capture,
		}

		server := smtp.NewServer(backend)
		server.Domain = "mailvoid.local"
		server.MaxMessageBytes = s.cfg.MaxMessageBytes
		server.ReadTimeout = s.cfg.ReadTimeout
		server.WriteTimeout = s.cfg.ReadTimeout
		server.ErrorLog = logger

		listener, err := net.Listen("tcp", addr)
		if err != nil {
			if logFile != nil {
				_ = logFile.Close()
			}
			s.closeLocked()
			return fmt.Errorf("listen SMTP %s (%s): %w", addr, svcCfg.LogName(), err)
		}

		svc := &service{
			cfg:      svcCfg,
			backend:  backend,
			server:   server,
			listener: listener,
			logFile:  logFile,
		}
		s.services = append(s.services, svc)

		logger.Printf("listening addr=%s name=%s aliases=%v", addr, svcCfg.LogName(), svcCfg.Aliases)
		go s.serve(svc, addr)
	}
	return nil
}

func (s *Server) serve(svc *service, addr string) {
	if err := svc.server.Serve(svc.listener); err != nil && !errors.Is(err, smtp.ErrServerClosed) {
		svc.backend.logf("serve error addr=%s error=%v", addr, err)
		s.bus.Emit(events.New(events.TypeSMTPError, events.SeverityError, map[string]any{
			"error":      err.Error(),
			"local_port": svc.cfg.Port,
			"service":    svc.cfg.LogName(),
		}))
	}
}

// Addrs returns the bound listener addresses (useful when port 0 is used).
func (s *Server) Addrs() []net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]net.Addr, len(s.services))
	for i, svc := range s.services {
		out[i] = svc.listener.Addr()
	}
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
	for _, svc := range s.services {
		if err := svc.server.Close(); err != nil && !errors.Is(err, smtp.ErrServerClosed) && firstErr == nil {
			firstErr = err
		}
		_ = svc.listener.Close()
		if svc.backend.logger != nil {
			svc.backend.logf("stopped addr=%s", svc.cfg.Address())
		}
		if svc.logFile != nil {
			_ = svc.logFile.Close()
		}
	}
	return firstErr
}

func openServiceLog(cfg config.Config, svc config.SMTPService) (*log.Logger, *os.File, error) {
	if !cfg.LogEnabled {
		return log.New(io.Discard, "", 0), nil, nil
	}
	path := svc.LogPath
	if path == "" {
		path = filepath.Join(cfg.LogDir, svc.LogName()+".log")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, err
	}
	return log.New(file, "["+svc.LogName()+"] ", log.LstdFlags|log.Lmicroseconds), file, nil
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
