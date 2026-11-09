// Command mailvoid-it is a standalone integration client for MailVoid. It can
// run as an interactive Charmbracelet TUI or as a batch test runner. Tests are
// JTD-validated JSON files under testdata/integration.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
)

type options struct {
	batch         bool
	suiteDir      string
	serverPath    string
	baseURL       string
	smtpAddr      string
	adminUser     string
	adminPassword string
	validateOnly  bool
	list          bool
	keep          bool
	verbose       bool
	filter        string
}

func main() {
	var opts options
	flag.BoolVar(&opts.batch, "batch", false, "run in batch mode (no TUI)")
	flag.StringVar(&opts.suiteDir, "suite", "testdata/integration", "directory containing JTD test files")
	flag.StringVar(&opts.serverPath, "server", "bin/mailvoid", "path to the MailVoid server binary")
	flag.StringVar(&opts.baseURL, "base-url", "", "use an external MailVoid HTTP endpoint instead of launching a server")
	flag.StringVar(&opts.smtpAddr, "smtp-addr", "", "external SMTP host:port (required with -base-url)")
	flag.StringVar(&opts.adminUser, "admin-user", "admin", "administrator username")
	flag.StringVar(&opts.adminPassword, "admin-password", "mailvoid-it-password", "administrator password")
	flag.BoolVar(&opts.validateOnly, "validate-only", false, "validate the JTD suite without running tests")
	flag.BoolVar(&opts.list, "list", false, "list tests and exit")
	flag.BoolVar(&opts.keep, "keep", false, "keep the temporary server data directory")
	flag.BoolVar(&opts.verbose, "verbose", false, "show all step logs in batch mode")
	flag.StringVar(&opts.filter, "test", "", "comma-separated test name or file filter")
	flag.Parse()

	suite, err := LoadSuite(opts.suiteDir)
	if err != nil {
		fail(err)
	}
	tests := filterTests(suite.Tests, opts.filter)
	if len(tests) == 0 {
		fail(fmt.Errorf("no tests match %q", opts.filter))
	}
	if opts.list {
		for _, test := range tests {
			fmt.Printf("%-32s %s\n", test.Definition.Name, test.File)
		}
		return
	}
	if opts.validateOnly {
		fmt.Printf("✔ JTD suite valid: %d test(s) in %s\n", len(tests), suite.Dir)
		return
	}

	if opts.baseURL != "" && opts.smtpAddr == "" {
		fail(fmt.Errorf("-smtp-addr is required with -base-url"))
	}
	server, err := StartServer(Options{
		ServerPath:    opts.serverPath,
		SuiteDir:      opts.suiteDir,
		BaseURL:       opts.baseURL,
		SMTPAddr:      opts.smtpAddr,
		AdminUser:     opts.adminUser,
		AdminPassword: opts.adminPassword,
		Keep:          opts.keep,
		Verbose:       opts.verbose,
	})
	if err != nil {
		fail(err)
	}
	defer func() {
		_ = server.Stop()
		server.Cleanup()
	}()

	interactive := !opts.batch && term.IsTerminal(os.Stdout.Fd())
	if interactive {
		if err := runTUI(server, Options{AdminUser: opts.adminUser, AdminPassword: opts.adminPassword}, tests); err != nil {
			fail(err)
		}
		return
	}

	if err := runBatch(server, Options{AdminUser: opts.adminUser, AdminPassword: opts.adminPassword}, tests, opts.verbose); err != nil {
		fail(err)
	}
}

func filterTests(tests []SuiteTest, filter string) []SuiteTest {
	if strings.TrimSpace(filter) == "" {
		return tests
	}
	parts := strings.Split(filter, ",")
	needles := make([]string, 0, len(parts))
	for _, part := range parts {
		needles = append(needles, strings.ToLower(strings.TrimSpace(part)))
	}
	var out []SuiteTest
	for _, test := range tests {
		for _, needle := range needles {
			if strings.Contains(strings.ToLower(test.Definition.Name), needle) ||
				strings.Contains(strings.ToLower(test.File), needle) {
				out = append(out, test)
				break
			}
		}
	}
	return out
}

func runBatch(server *Server, opts Options, tests []SuiteTest, verbose bool) error {
	started := time.Now()
	printer := newBatchPrinter(verbose)
	printer.Header(server.BaseURL, len(tests))
	ctx := context.Background()
	passed, failed := 0, 0
	for _, test := range tests {
		harness, err := NewHarness(server, opts)
		if err != nil {
			printer.Test(TestResult{Name: test.Definition.Name, File: test.File, Err: err})
			failed++
			continue
		}
		result := harness.RunTest(ctx, test)
		harness.Close()
		printer.Test(result)
		if result.Passed {
			passed++
		} else {
			failed++
		}
	}
	printer.Summary(passed, failed, time.Since(started))
	if failed > 0 {
		return fmt.Errorf("%d test(s) failed", failed)
	}
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "mailvoid-it: "+err.Error())
	os.Exit(1)
}
