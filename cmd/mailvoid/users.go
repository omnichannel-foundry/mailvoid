package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/gnrfan/mailvoid/internal/auth"
	"github.com/gnrfan/mailvoid/internal/config"
	"github.com/gnrfan/mailvoid/internal/storage"
)

const usersHelp = `Usage:
  mailvoid users list [flags]
  mailvoid users passwd [username] [flags]

Commands:
  list      List administrator accounts.
  passwd    Set an administrator password. If username is omitted, the
            configured administrator username is used.

Flags are the same as the server, notably -config, -data-dir, and -db-path.
The MailVoid server must be stopped while these commands run.`

func runUserCommand(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	switch args[0] {
	case "users", "user":
		return true, runUsers(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return true, nil
	default:
		return false, nil
	}
}

func printUsage() {
	fmt.Print(`MailVoid - wildcard SMTP capture server

Usage:
  mailvoid [server flags]
  mailvoid users list [flags]
  mailvoid users passwd [username] [flags]
  mailvoid --version

Run "mailvoid users help" for user administration details.
`)
}

func runUsers(args []string) error {
	if len(args) == 0 {
		return errors.New(usersHelp)
	}
	switch args[0] {
	case "list", "ls":
		return runUsersList(args[1:])
	case "passwd", "password", "set-password":
		return runUsersPasswd(args[1:])
	case "help", "-h", "--help":
		fmt.Println(usersHelp)
		return nil
	default:
		return fmt.Errorf("unknown users subcommand %q\n\n%s", args[0], usersHelp)
	}
}

func runUsersList(args []string) error {
	_, store, err := openCLIStore(args)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	return listUsers(context.Background(), store, os.Stdout)
}

func runUsersPasswd(args []string) error {
	username := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		username = args[0]
		args = args[1:]
	}

	cfg, store, err := openCLIStore(args)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	if username == "" {
		username = cfg.AdminUser
	}
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return errors.New("username is required")
	}

	reader := newPasswordReader(os.Stdin)
	password, err := reader.read("New password: ")
	if err != nil {
		return err
	}
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	confirm, err := reader.read("Confirm password: ")
	if err != nil {
		return err
	}
	if password != confirm {
		return errors.New("passwords do not match")
	}

	if err := changeUserPassword(context.Background(), store, username, password); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("user %q not found", username)
		}
		return err
	}
	fmt.Printf("Password updated for %s.\n", username)
	return nil
}

func openCLIStore(args []string) (config.Config, storage.Store, error) {
	cfg, err := config.Load(args, os.Getenv)
	if err != nil {
		return config.Config{}, nil, err
	}
	store, err := storage.NewGenji(cfg.DBPath)
	if err != nil {
		return config.Config{}, nil, fmt.Errorf("open database %s (stop the mailvoid server first if it is running): %w", cfg.DBPath, err)
	}
	return cfg, store, nil
}

func listUsers(ctx context.Context, store storage.Store, out io.Writer) error {
	users, err := store.ListUsers(ctx)
	if err != nil {
		return err
	}
	if len(users) == 0 {
		_, err := fmt.Fprintln(out, "No administrator users found.")
		return err
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "USERNAME\tCREATED"); err != nil {
		return err
	}
	for _, user := range users {
		created := "-"
		if !user.CreatedAt.IsZero() {
			created = user.CreatedAt.UTC().Format(time.RFC3339)
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\n", user.Username, created); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func changeUserPassword(ctx context.Context, store storage.Store, username, password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	return auth.New(store).SetPassword(ctx, strings.ToLower(strings.TrimSpace(username)), password)
}

type passwordReader struct {
	fd       uintptr
	terminal bool
	in       *bufio.Reader
}

func newPasswordReader(file *os.File) *passwordReader {
	return &passwordReader{
		fd:       file.Fd(),
		terminal: term.IsTerminal(file.Fd()),
		in:       bufio.NewReader(file),
	}
}

func (r *passwordReader) read(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	if r.terminal {
		data, err := term.ReadPassword(r.fd)
		fmt.Fprintln(os.Stderr)
		return string(data), err
	}
	line, err := r.in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
