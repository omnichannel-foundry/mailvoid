package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type fileConfig struct {
	Hostname  *string      `toml:"hostname"`
	Scheme    *string      `toml:"scheme"`
	PublicURL *string      `toml:"public_url"`
	Domains   []string     `toml:"domains"`
	Web       *fileWeb     `toml:"web"`
	SMTP      *fileSMTP    `toml:"smtp"`
	Events    *fileEvents  `toml:"events"`
	Logging   *fileLogging `toml:"logging"`
	Storage   *fileStorage `toml:"storage"`
	Search    *fileSearch  `toml:"search"`
	Auth      *fileAuth    `toml:"auth"`
	Limits    *fileLimits  `toml:"limits"`
}

type fileWeb struct {
	Addr *string `toml:"addr"`
	Bind *string `toml:"bind"`
	Port *int    `toml:"port"`
}

type fileSMTP struct {
	Hosts    []string          `toml:"hosts"`
	Ports    []int             `toml:"ports"`
	Domains  []string          `toml:"domains"`
	Services []fileSMTPService `toml:"services"`
}

type fileSMTPService struct {
	Name    string   `toml:"name"`
	Host    string   `toml:"host"`
	Port    int      `toml:"port"`
	Aliases []string `toml:"aliases"`
	LogPath string   `toml:"log_path"`
}

type fileEvents struct {
	JSONEnabled *bool   `toml:"json_enabled"`
	JSONPath    *string `toml:"json_path"`
}

type fileLogging struct {
	Enabled *bool   `toml:"enabled"`
	Dir     *string `toml:"dir"`
}

type fileStorage struct {
	DataDir *string `toml:"data_dir"`
	DBPath  *string `toml:"db_path"`
}

type fileSearch struct {
	Enabled   *bool   `toml:"enabled"`
	IndexPath *string `toml:"index_path"`
}

type fileAuth struct {
	AdminUser          *string `toml:"admin_user"`
	AdminPassword      *string `toml:"admin_password"`
	AdminResetPassword *bool   `toml:"admin_reset_password"`
	APIToken           *string `toml:"api_token"`
	WebDAVUser         *string `toml:"webdav_user"`
	WebDAVPassword     *string `toml:"webdav_password"`
	CookieSecure       *bool   `toml:"cookie_secure"`
}

type fileLimits struct {
	MaxMessageBytes *int64         `toml:"max_message_bytes"`
	ReadTimeout     *time.Duration `toml:"read_timeout"`
	SweepInterval   *time.Duration `toml:"sweep_interval"`
}

func applyFile(c *Config, path string, required bool) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) && !required {
			return nil
		}
		return fmt.Errorf("read config %s: %w", path, err)
	}

	var fc fileConfig
	md, err := toml.DecodeFile(path, &fc)
	if err != nil {
		return fmt.Errorf("parse config %s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, key := range undecoded {
			keys[i] = key.String()
		}
		return fmt.Errorf("config %s: unknown keys: %s", path, strings.Join(keys, ", "))
	}

	if fc.Hostname != nil {
		c.Hostname = *fc.Hostname
	}
	if fc.Scheme != nil {
		c.Scheme = *fc.Scheme
	}
	if fc.PublicURL != nil {
		c.PublicURL = *fc.PublicURL
	}
	if fc.Domains != nil {
		c.Domains = parseList(strings.Join(fc.Domains, ","))
	}

	if fc.Web != nil {
		if fc.Web.Addr != nil {
			c.HTTPAddr = *fc.Web.Addr
		}
		if fc.Web.Bind != nil || fc.Web.Port != nil {
			host, port := splitHTTPAddr(c.HTTPAddr)
			if fc.Web.Bind != nil {
				host = *fc.Web.Bind
			}
			if fc.Web.Port != nil {
				port = *fc.Web.Port
			}
			c.HTTPAddr = joinHTTPAddr(host, port)
		}
	}

	if fc.SMTP != nil {
		if len(fc.SMTP.Ports) > 0 {
			c.SMTPPorts = append([]int(nil), fc.SMTP.Ports...)
		}
		if fc.SMTP.Domains != nil {
			c.Domains = parseList(strings.Join(fc.SMTP.Domains, ","))
		}
		switch {
		case len(fc.SMTP.Services) > 0:
			c.SMTPServices = make([]SMTPService, 0, len(fc.SMTP.Services))
			for _, svc := range fc.SMTP.Services {
				c.SMTPServices = append(c.SMTPServices, SMTPService{
					Name:    svc.Name,
					Host:    svc.Host,
					Port:    svc.Port,
					Aliases: append([]string(nil), svc.Aliases...),
					LogPath: svc.LogPath,
				})
			}
		case len(fc.SMTP.Hosts) > 0:
			c.SMTPServices = servicesFromHostsPorts(fc.SMTP.Hosts, c.SMTPPorts)
		case len(fc.SMTP.Ports) > 0:
			c.SMTPServices = nil
		}
	}

	if fc.Events != nil {
		if fc.Events.JSONEnabled != nil {
			c.EventLogDisabled = !*fc.Events.JSONEnabled
		}
		if fc.Events.JSONPath != nil {
			c.EventLogPath = *fc.Events.JSONPath
		}
	}
	if fc.Logging != nil {
		if fc.Logging.Enabled != nil {
			c.LogEnabled = *fc.Logging.Enabled
		}
		if fc.Logging.Dir != nil {
			c.LogDir = *fc.Logging.Dir
		}
	}
	if fc.Storage != nil {
		if fc.Storage.DataDir != nil {
			c.DataDir = *fc.Storage.DataDir
		}
		if fc.Storage.DBPath != nil {
			c.DBPath = *fc.Storage.DBPath
		}
	}
	if fc.Search != nil {
		if fc.Search.Enabled != nil {
			c.SearchEnabled = *fc.Search.Enabled
		}
		if fc.Search.IndexPath != nil {
			c.SearchIndexPath = *fc.Search.IndexPath
		}
	}
	if fc.Auth != nil {
		if fc.Auth.AdminUser != nil {
			c.AdminUser = *fc.Auth.AdminUser
		}
		if fc.Auth.AdminPassword != nil {
			c.AdminPassword = *fc.Auth.AdminPassword
		}
		if fc.Auth.AdminResetPassword != nil {
			c.AdminResetPassword = *fc.Auth.AdminResetPassword
		}
		if fc.Auth.APIToken != nil {
			c.APIToken = *fc.Auth.APIToken
		}
		if fc.Auth.WebDAVUser != nil {
			c.WebDAVUser = *fc.Auth.WebDAVUser
		}
		if fc.Auth.WebDAVPassword != nil {
			c.WebDAVPassword = *fc.Auth.WebDAVPassword
		}
		if fc.Auth.CookieSecure != nil {
			c.SessionCookieSecure = *fc.Auth.CookieSecure
		}
	}
	if fc.Limits != nil {
		if fc.Limits.MaxMessageBytes != nil {
			c.MaxMessageBytes = *fc.Limits.MaxMessageBytes
		}
		if fc.Limits.ReadTimeout != nil {
			c.ReadTimeout = *fc.Limits.ReadTimeout
		}
		if fc.Limits.SweepInterval != nil {
			c.SweepInterval = *fc.Limits.SweepInterval
		}
	}
	return nil
}
