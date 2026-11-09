// Command lorem-mail-generator sends random Lorem Ipsum messages to a MailVoid SMTP
// listener for load, volume, and UI testing.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type options struct {
	addr            string
	from            string
	fromAddr        string
	to              string
	recipients      []string
	count           int
	minWords        int
	maxWords        int
	concurrency     int
	delay           time.Duration
	timeout         time.Duration
	seed            int64
	html            bool
	subjectPrefix   string
	failFast        bool
	quiet           bool
	randomMailboxes int
	mailboxPrefix   string
	domains         []string
	randomRecipient bool
}

func main() {
	opts, err := parseOptions(os.Args[1:])
	if err != nil {
		if err == flag.ErrHelp {
			return
		}
		fmt.Fprintln(os.Stderr, "lorem-mail-generator:", err)
		os.Exit(2)
	}
	if err := run(opts, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "lorem-mail-generator:", err)
		os.Exit(1)
	}
}

func parseOptions(args []string) (options, error) {
	var opts options
	fs := flag.NewFlagSet("lorem-mail-generator", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Send random Lorem Ipsum messages to a MailVoid SMTP listener.")
		fmt.Fprintln(fs.Output(), "")
		fmt.Fprintln(fs.Output(), "Usage:")
		fmt.Fprintln(fs.Output(), "  lorem-mail-generator [flags]")
		fmt.Fprintln(fs.Output(), "")
		fmt.Fprintln(fs.Output(), "Flags:")
		fs.PrintDefaults()
	}

	fs.StringVar(&opts.addr, "addr", "127.0.0.1:1025", "SMTP server address")
	fs.StringVar(&opts.from, "from", "lorem@example.test", "envelope sender")
	fs.StringVar(&opts.to, "to", "inbox@example.test", "comma-separated recipients")
	fs.IntVar(&opts.count, "count", 10, "number of messages to send")
	fs.IntVar(&opts.minWords, "min-words", 10, "minimum body word count")
	fs.IntVar(&opts.maxWords, "max-words", 120, "maximum body word count")
	fs.IntVar(&opts.concurrency, "concurrency", 1, "parallel SMTP connections")
	fs.DurationVar(&opts.delay, "delay", 0, "delay between messages per worker")
	fs.DurationVar(&opts.timeout, "timeout", 10*time.Second, "per-connection timeout")
	fs.Int64Var(&opts.seed, "seed", 0, "random seed (0 uses the current time)")
	fs.BoolVar(&opts.html, "html", false, "send HTML bodies instead of plain text")
	fs.StringVar(&opts.subjectPrefix, "subject-prefix", "Lorem", "subject prefix")
	fs.BoolVar(&opts.failFast, "fail-fast", false, "stop at the first send error")
	fs.BoolVar(&opts.quiet, "quiet", false, "suppress progress output")
	fs.IntVar(&opts.randomMailboxes, "random-mailboxes", 0, "generate N random recipient mailboxes (overrides -to)")
	fs.StringVar(&opts.mailboxPrefix, "mailbox-prefix", "lorem", "local-part prefix for random mailboxes")
	domainList := ""
	fs.StringVar(&domainList, "domains", "example.test", "comma-separated domains for random mailboxes")

	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if opts.addr == "" {
		return options{}, fmt.Errorf("-addr is required")
	}
	if opts.count < 1 {
		return options{}, fmt.Errorf("-count must be at least 1")
	}
	if opts.minWords < 1 {
		return options{}, fmt.Errorf("-min-words must be at least 1")
	}
	if opts.maxWords < opts.minWords {
		return options{}, fmt.Errorf("-max-words must be greater than or equal to -min-words")
	}
	if opts.concurrency < 1 {
		return options{}, fmt.Errorf("-concurrency must be at least 1")
	}
	if opts.timeout <= 0 {
		return options{}, fmt.Errorf("-timeout must be positive")
	}
	if opts.delay < 0 {
		return options{}, fmt.Errorf("-delay cannot be negative")
	}
	if opts.randomMailboxes < 0 {
		return options{}, fmt.Errorf("-random-mailboxes cannot be negative")
	}
	if opts.randomMailboxes > 0 {
		domains, err := parseDomains(domainList)
		if err != nil {
			return options{}, err
		}
		opts.domains = domains
	}

	from, err := mail.ParseAddress(strings.TrimSpace(opts.from))
	if err != nil {
		return options{}, fmt.Errorf("invalid -from address: %w", err)
	}
	recipients, err := mail.ParseAddressList(strings.TrimSpace(opts.to))
	if err != nil {
		return options{}, fmt.Errorf("invalid -to address list: %w", err)
	}
	if len(recipients) == 0 {
		return options{}, fmt.Errorf("-to must contain at least one address")
	}
	opts.fromAddr = from.Address
	opts.recipients = make([]string, len(recipients))
	for i, recipient := range recipients {
		opts.recipients[i] = recipient.Address
	}
	return opts, nil
}

func parseDomains(value string) ([]string, error) {
	seen := make(map[string]bool)
	var out []string
	for _, part := range strings.Split(value, ",") {
		domain := strings.ToLower(strings.TrimSpace(part))
		if domain == "" || seen[domain] {
			continue
		}
		if _, err := mail.ParseAddress("lorem@" + domain); err != nil {
			return nil, fmt.Errorf("invalid domain %q: %w", domain, err)
		}
		seen[domain] = true
		out = append(out, domain)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("-domains must contain at least one domain")
	}
	return out, nil
}

func buildMailboxPool(rng *rand.Rand, count int, domains []string, prefix string) []string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "lorem"
	}
	seen := make(map[string]bool, count)
	out := make([]string, 0, count)
	for len(out) < count {
		word := loremWords[rng.Intn(len(loremWords))]
		domain := domains[rng.Intn(len(domains))]
		address := fmt.Sprintf("%s-%s-%04d@%s", prefix, word, rng.Intn(10000), domain)
		if seen[address] {
			continue
		}
		seen[address] = true
		out = append(out, address)
	}
	return out
}

func run(opts options, out, errOut io.Writer) error {
	if opts.seed == 0 {
		opts.seed = time.Now().UnixNano()
	}
	if opts.randomMailboxes > 0 {
		opts.recipients = buildMailboxPool(rand.New(rand.NewSource(opts.seed)), opts.randomMailboxes, opts.domains, opts.mailboxPrefix)
		opts.randomRecipient = true
	}
	if !opts.quiet {
		to := strings.Join(opts.recipients, ",")
		if opts.randomRecipient {
			to = fmt.Sprintf("random(%d mailboxes)", len(opts.recipients))
		}
		fmt.Fprintf(out, "lorem-mail-generator: addr=%s from=%s to=%s count=%d words=%d-%d concurrency=%d seed=%d\n",
			opts.addr, opts.fromAddr, to, opts.count,
			opts.minWords, opts.maxWords, opts.concurrency, opts.seed)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var next, sent, failed atomic.Int64
	start := time.Now()
	var wg sync.WaitGroup
	for workerID := 0; workerID < opts.concurrency; workerID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runWorker(ctx, cancel, opts, id, &next, &sent, &failed, errOut)
		}(workerID)
	}
	wg.Wait()
	elapsed := time.Since(start)
	rate := float64(sent.Load()) / elapsed.Seconds()
	if !opts.quiet {
		fmt.Fprintf(out, "lorem-mail-generator: sent=%d failed=%d elapsed=%s rate=%.1f/s seed=%d\n",
			sent.Load(), failed.Load(), elapsed.Round(time.Millisecond), rate, opts.seed)
	}
	if failed.Load() > 0 {
		return fmt.Errorf("%d message(s) failed", failed.Load())
	}
	return nil
}

func runWorker(ctx context.Context, cancel context.CancelFunc, opts options, workerID int, next, sent, failed *atomic.Int64, errOut io.Writer) {
	rng := rand.New(rand.NewSource(opts.seed + int64(workerID)*7919))
	for {
		if ctx.Err() != nil {
			return
		}
		index := int(next.Add(1)) - 1
		if index >= opts.count {
			return
		}
		recipients := opts.recipients
		toHeader := opts.to
		if opts.randomRecipient {
			toHeader = opts.recipients[rng.Intn(len(opts.recipients))]
			recipients = []string{toHeader}
		}
		message := buildMessage(opts, index, rng, toHeader)
		if err := sendMessage(opts, message, recipients); err != nil {
			failed.Add(1)
			if !opts.quiet {
				fmt.Fprintf(errOut, "lorem-mail-generator: message %d failed: %v\n", index, err)
			}
			if opts.failFast {
				cancel()
				return
			}
		} else {
			sent.Add(1)
		}
		if opts.delay > 0 {
			select {
			case <-time.After(opts.delay):
			case <-ctx.Done():
				return
			}
		}
	}
}

func sendMessage(opts options, message []byte, recipients []string) error {
	conn, err := net.DialTimeout("tcp", opts.addr, opts.timeout)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(opts.timeout)); err != nil {
		return err
	}

	host, _, err := net.SplitHostPort(opts.addr)
	if err != nil || host == "" {
		host = "localhost"
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if err := client.Mail(opts.fromAddr); err != nil {
		return err
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func buildMessage(opts options, index int, rng *rand.Rand, toHeader string) []byte {
	words := opts.minWords
	if opts.maxWords > opts.minWords {
		words += rng.Intn(opts.maxWords - opts.minWords + 1)
	}
	body := loremBody(rng, words, opts.html)
	subject := loremSubject(rng, opts.subjectPrefix)
	now := time.Now().UTC()

	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", opts.from)
	fmt.Fprintf(&b, "To: %s\r\n", toHeader)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	fmt.Fprintf(&b, "Date: %s\r\n", now.Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <lorem-%d-%d-%d@mailvoid.local>\r\n", now.UnixNano(), index, rng.Int63())
	fmt.Fprintf(&b, "X-Mailvoid-Lorem-Index: %d\r\n", index)
	fmt.Fprintf(&b, "X-Mailvoid-Lorem-Words: %d\r\n", words)
	if opts.html {
		b.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	} else {
		b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	}
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

func loremBody(rng *rand.Rand, totalWords int, html bool) string {
	if totalWords < 1 {
		totalWords = 1
	}
	var b strings.Builder
	if html {
		b.WriteString("<html><body>\n")
	}
	written := 0
	for written < totalWords {
		remaining := totalWords - written
		sentenceLen := 5 + rng.Intn(11)
		if sentenceLen > remaining {
			sentenceLen = remaining
		}
		sentence := make([]string, sentenceLen)
		for i := range sentence {
			sentence[i] = loremWords[rng.Intn(len(loremWords))]
		}
		sentence[0] = capitalize(sentence[0])
		line := strings.Join(sentence, " ") + "."
		if html {
			fmt.Fprintf(&b, "<p>%s</p>\n", line)
		} else {
			b.WriteString(line)
			b.WriteString("\n\n")
		}
		written += sentenceLen
	}
	if html {
		b.WriteString("</body></html>\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func loremSubject(rng *rand.Rand, prefix string) string {
	count := 3 + rng.Intn(5)
	words := make([]string, count)
	for i := range words {
		words[i] = loremWords[rng.Intn(len(loremWords))]
	}
	words[0] = capitalize(words[0])
	subject := strings.Join(words, " ")
	if prefix = strings.TrimSpace(prefix); prefix != "" {
		subject = prefix + ": " + subject
	}
	return subject
}

func capitalize(word string) string {
	if word == "" {
		return word
	}
	return strings.ToUpper(word[:1]) + word[1:]
}

var loremWords = []string{
	"lorem", "ipsum", "dolor", "sit", "amet", "consectetur", "adipiscing", "elit",
	"sed", "do", "eiusmod", "tempor", "incididunt", "ut", "labore", "et", "dolore",
	"magna", "aliqua", "enim", "ad", "minim", "veniam", "quis", "nostrud",
	"exercitation", "ullamco", "laboris", "nisi", "aliquip", "ex", "ea", "commodo",
	"consequat", "duis", "aute", "irure", "in", "reprehenderit", "voluptate",
	"velit", "esse", "cillum", "eu", "fugiat", "nulla", "pariatur", "excepteur",
	"sint", "occaecat", "cupidatat", "non", "proident", "sunt", "culpa", "qui",
	"officia", "deserunt", "mollit", "anim", "id", "est", "laborum", "vestibulum",
	"curabitur", "phasellus", "integer", "facilisis", "malesuada", "tincidunt",
	"egestas", "suspendisse", "pellentesque", "habitant", "morbi", "tristique",
	"senectus", "netus", "fames", "turpis", "fermentum", "posuere", "urna", "nec",
	"praesent", "semper", "feugiat", "nibh", "elementum", "sagittis", "vitae",
	"magnis", "dis", "parturient", "montes", "nascetur", "ridiculus", "mus",
	"natoque", "penatibus", "vivamus", "arcu", "felis", "blandit", "volutpat",
}
