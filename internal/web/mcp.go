package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/mailbox"
	"github.com/gnrfan/mailvoid/internal/models"
	searchidx "github.com/gnrfan/mailvoid/internal/search"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCP output types intentionally use strings for timestamps so the generated
// JSON schemas stay simple and portable across agent clients.

type mcpMailboxSummary struct {
	Address          string `json:"address" jsonschema:"mailbox@domain address"`
	Mailbox          string `json:"mailbox" jsonschema:"local part"`
	Domain           string `json:"domain" jsonschema:"recipient domain"`
	MessageCount     int    `json:"message_count" jsonschema:"number of captured messages"`
	LatestMessageID  string `json:"latest_message_id,omitempty" jsonschema:"most recent message id"`
	LatestSubject    string `json:"latest_subject,omitempty" jsonschema:"subject of the most recent message"`
	LatestSender     string `json:"latest_sender,omitempty" jsonschema:"envelope sender of the most recent message"`
	LatestReceivedAt string `json:"latest_received_at,omitempty" jsonschema:"RFC 3339 receive time of the most recent message"`
}

func mcpMailboxFrom(summary mailbox.Summary) mcpMailboxSummary {
	out := mcpMailboxSummary{
		Address:         summary.Address,
		Mailbox:         summary.Mailbox,
		Domain:          summary.Domain,
		MessageCount:    summary.MessageCount,
		LatestMessageID: summary.LatestMessageID,
		LatestSubject:   summary.LatestSubject,
		LatestSender:    summary.LatestSender,
	}
	if summary.LatestReceivedAt != nil {
		out.LatestReceivedAt = summary.LatestReceivedAt.UTC().Format(time.RFC3339Nano)
	}
	return out
}

type mcpMessageSummary struct {
	ID         string `json:"id"`
	ReceivedAt string `json:"received_at"`
	RemoteIP   string `json:"remote_ip"`
	LocalPort  int    `json:"local_port"`
	Domain     string `json:"domain"`
	Mailbox    string `json:"mailbox"`
	Recipient  string `json:"recipient"`
	Sender     string `json:"sender"`
	Subject    string `json:"subject"`
	Size       int64  `json:"size"`
	Action     string `json:"action"`
	PolicyID   string `json:"policy_id,omitempty"`
	PolicyName string `json:"policy_name,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
}

func mcpMessageFrom(message *models.Message) mcpMessageSummary {
	out := mcpMessageSummary{
		ID:         message.ID,
		ReceivedAt: message.ReceivedAt.UTC().Format(time.RFC3339Nano),
		RemoteIP:   message.RemoteIP,
		LocalPort:  message.LocalPort,
		Domain:     message.Domain,
		Mailbox:    message.Mailbox,
		Recipient:  message.Recipient,
		Sender:     message.Sender,
		Subject:    message.Subject,
		Size:       message.Size,
		Action:     message.Action,
		PolicyID:   message.PolicyID,
		PolicyName: message.PolicyName,
	}
	if message.ExpiresAt != nil {
		out.ExpiresAt = message.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	return out
}

type mcpMessageDetail struct {
	mcpMessageSummary
	Headers    map[string][]string `json:"headers,omitempty"`
	RawHeaders string              `json:"raw_headers,omitempty"`
	BodyText   string              `json:"body_text,omitempty"`
	BodyHTML   string              `json:"body_html,omitempty"`
	Raw        string              `json:"raw,omitempty"`
	Deleted    bool                `json:"deleted"`
}

func mcpDetailFrom(message *models.Message, includeRaw bool) mcpMessageDetail {
	out := mcpMessageDetail{
		mcpMessageSummary: mcpMessageFrom(message),
		Headers:           message.Headers,
		RawHeaders:        message.RawHeaders,
		BodyText:          message.BodyText,
		BodyHTML:          message.BodyHTML,
		Deleted:           message.Deleted,
	}
	if includeRaw {
		out.Raw = message.Raw
	}
	return out
}

type mcpPolicy struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Field      string `json:"field"`
	Pattern    string `json:"pattern"`
	Action     string `json:"action"`
	TTLSeconds int64  `json:"ttl_seconds"`
	Priority   int    `json:"priority"`
	Enabled    bool   `json:"enabled"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

func mcpPolicyFrom(policy *models.Policy) mcpPolicy {
	return mcpPolicy{
		ID:         policy.ID,
		Name:       policy.Name,
		Field:      policy.Field,
		Pattern:    policy.Pattern,
		Action:     policy.Action,
		TTLSeconds: policy.TTLSeconds,
		Priority:   policy.Priority,
		Enabled:    policy.Enabled,
		CreatedAt:  policy.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:  policy.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

type mcpEvent struct {
	ID        string         `json:"id"`
	Timestamp string         `json:"timestamp"`
	Type      string         `json:"type"`
	Severity  string         `json:"severity"`
	Payload   map[string]any `json:"payload,omitempty"`
}

func mcpEventFrom(event events.Event) mcpEvent {
	return mcpEvent{
		ID:        event.ID,
		Timestamp: event.Timestamp.UTC().Format(time.RFC3339Nano),
		Type:      event.Type,
		Severity:  event.Severity,
		Payload:   event.Payload,
	}
}

type listMailboxesInput struct {
	Domain string `json:"domain,omitempty" jsonschema:"optional exact recipient domain"`
	Since  string `json:"since,omitempty" jsonschema:"optional RFC 3339 lower bound"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum mailboxes to return (default 50, max 200)"`
	Offset int    `json:"offset,omitempty" jsonschema:"zero-based offset"`
}

type listMailboxesOutput struct {
	Mailboxes []mcpMailboxSummary `json:"mailboxes"`
	Count     int                 `json:"count"`
}

type checkMailboxInput struct {
	Address string `json:"address" jsonschema:"mailbox address such as inbox@example.test"`
	Since   string `json:"since,omitempty" jsonschema:"optional RFC 3339 lower bound"`
	Limit   int    `json:"limit,omitempty" jsonschema:"recent messages to include (default 10, max 100)"`
}

type checkMailboxOutput struct {
	Found    bool                `json:"found" jsonschema:"true when the mailbox has at least one captured message"`
	Mailbox  mcpMailboxSummary   `json:"mailbox"`
	Messages []mcpMessageSummary `json:"messages"`
}

type waitForEmailInput struct {
	Address        string `json:"address" jsonschema:"mailbox address to watch"`
	Since          string `json:"since,omitempty" jsonschema:"optional RFC 3339 lower bound; defaults to now"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"wait timeout in seconds (default 30, max 60)"`
}

type waitForEmailOutput struct {
	Found         bool               `json:"found"`
	WaitedSeconds float64            `json:"waited_seconds"`
	Message       *mcpMessageSummary `json:"message,omitempty"`
}

type getMessageInput struct {
	ID         string `json:"id" jsonschema:"message id"`
	IncludeRaw bool   `json:"include_raw,omitempty" jsonschema:"include the complete raw RFC 5322 message"`
}

type getMessageOutput struct {
	Message mcpMessageDetail `json:"message"`
}

type searchMessagesInput struct {
	Query     string `json:"q,omitempty" jsonschema:"full-text query over subject, body, sender, recipient, mailbox, domain, and headers"`
	Domain    string `json:"domain,omitempty" jsonschema:"exact recipient domain"`
	Mailbox   string `json:"mailbox,omitempty" jsonschema:"exact recipient local part"`
	Recipient string `json:"recipient,omitempty" jsonschema:"exact mailbox@domain recipient"`
	Sender    string `json:"sender,omitempty" jsonschema:"exact envelope sender"`
	Action    string `json:"action,omitempty" jsonschema:"keep, ttl, or drop"`
	Port      int    `json:"port,omitempty" jsonschema:"receiving SMTP port"`
	Since     string `json:"since,omitempty" jsonschema:"RFC 3339 lower bound"`
	Until     string `json:"until,omitempty" jsonschema:"RFC 3339 upper bound"`
	Limit     int    `json:"limit,omitempty" jsonschema:"maximum messages (default 50, max 200)"`
	Offset    int    `json:"offset,omitempty" jsonschema:"zero-based offset"`
	Sort      string `json:"sort,omitempty" jsonschema:"relevance (default) or received_at"`
}

type searchMessagesOutput struct {
	Messages   []mcpMessageSummary            `json:"messages"`
	Count      int                            `json:"count"`
	Highlights map[string]map[string][]string `json:"highlights,omitempty"`
	Backend    string                         `json:"backend"`
}

type listEventsInput struct {
	Type     string `json:"type,omitempty" jsonschema:"exact event type"`
	Severity string `json:"severity,omitempty" jsonschema:"debug, info, warn, or error"`
	Query    string `json:"q,omitempty" jsonschema:"substring search over type, severity, and payload"`
	Since    string `json:"since,omitempty" jsonschema:"RFC 3339 lower bound"`
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum events (default 50, max 200)"`
	Offset   int    `json:"offset,omitempty" jsonschema:"zero-based offset"`
}

type listEventsOutput struct {
	Events []mcpEvent `json:"events"`
	Count  int        `json:"count"`
}

type listPoliciesInput struct{}

type listPoliciesOutput struct {
	Policies []mcpPolicy `json:"policies"`
	Count    int         `json:"count"`
}

type getStatsInput struct{}

type getStatsOutput struct {
	Messages    int    `json:"messages"`
	Messages24h int    `json:"messages_24h"`
	Policies    int    `json:"policies"`
	Events      int    `json:"events"`
	Version     string `json:"version"`
	Uptime      string `json:"uptime"`
}

type deleteMessageInput struct {
	ID string `json:"id" jsonschema:"message id to delete"`
}

type deleteMessageOutput struct {
	Deleted bool   `json:"deleted"`
	ID      string `json:"id"`
}

func (s *Server) newMCPHandler() http.Handler {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mailvoid",
		Title:   "MailVoid",
		Version: s.version,
	}, &mcp.ServerOptions{
		Instructions: "MailVoid captures inbound SMTP mail. Use list_mailboxes, " +
			"check_mailbox, and wait_for_email to verify delivery. Use " +
			"search_messages and get_message to inspect captured content. All " +
			"tools require the API token configured with MAILVOID_API_TOKEN.",
	})
	s.registerMCPTools(server)

	return mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{
			Stateless:    true,
			JSONResponse: true,
		},
	)
}

func (s *Server) registerMCPTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_mailboxes",
		Description: "List captured mailboxes with message counts and the latest message metadata.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listMailboxesInput) (*mcp.CallToolResult, listMailboxesOutput, error) {
		since, err := parseMCPTime(in.Since, "since")
		if err != nil {
			return nil, listMailboxesOutput{}, err
		}
		limit, offset := mcpPage(in.Limit, in.Offset)
		summaries, total, err := s.mailboxes.List(ctx, mailbox.ListOptions{
			Domain: strings.ToLower(strings.TrimSpace(in.Domain)),
			Since:  since,
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			return nil, listMailboxesOutput{}, err
		}
		out := make([]mcpMailboxSummary, 0, len(summaries))
		for _, summary := range summaries {
			out = append(out, mcpMailboxFrom(summary))
		}
		return nil, listMailboxesOutput{Mailboxes: out, Count: total}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "check_mailbox",
		Description: "Check whether mail was delivered to one mailbox and return recent messages.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in checkMailboxInput) (*mcp.CallToolResult, checkMailboxOutput, error) {
		since, err := parseMCPTime(in.Since, "since")
		if err != nil {
			return nil, checkMailboxOutput{}, err
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 10
		}
		if limit > 100 {
			limit = 100
		}
		summary, err := s.mailboxes.Summary(ctx, in.Address, since)
		if err != nil {
			return nil, checkMailboxOutput{}, err
		}
		messages, _, err := s.mailboxes.Messages(ctx, in.Address, mailbox.MessageOptions{Since: since, Limit: limit})
		if err != nil {
			return nil, checkMailboxOutput{}, err
		}
		out := checkMailboxOutput{
			Found:    summary.MessageCount > 0,
			Mailbox:  mcpMailboxFrom(summary),
			Messages: make([]mcpMessageSummary, 0, len(messages)),
		}
		for _, message := range messages {
			out.Messages = append(out.Messages, mcpMessageFrom(message))
		}
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "wait_for_email",
		Description: "Wait for a new message to arrive in a mailbox, or report that the timeout elapsed.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in waitForEmailInput) (*mcp.CallToolResult, waitForEmailOutput, error) {
		since, err := parseMCPTime(in.Since, "since")
		if err != nil {
			return nil, waitForEmailOutput{}, err
		}
		timeout := time.Duration(in.TimeoutSeconds) * time.Second
		if in.TimeoutSeconds <= 0 {
			timeout = 30 * time.Second
		}
		started := time.Now()
		message, err := s.mailboxes.Wait(ctx, in.Address, since, timeout)
		waited := time.Since(started).Seconds()
		if err != nil {
			if errors.Is(err, mailbox.ErrTimeout) {
				return nil, waitForEmailOutput{Found: false, WaitedSeconds: waited}, nil
			}
			return nil, waitForEmailOutput{}, err
		}
		summary := mcpMessageFrom(message)
		return nil, waitForEmailOutput{Found: true, WaitedSeconds: waited, Message: &summary}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_message",
		Description: "Get one captured message, including decoded bodies and headers. Set include_raw to return the raw RFC 5322 message.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getMessageInput) (*mcp.CallToolResult, getMessageOutput, error) {
		message, err := s.store.GetMessage(ctx, in.ID)
		if err != nil {
			return nil, getMessageOutput{}, errors.New("message not found")
		}
		return nil, getMessageOutput{Message: mcpDetailFrom(message, in.IncludeRaw)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_messages",
		Description: "Search captured messages with the same filters as the REST API.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchMessagesInput) (*mcp.CallToolResult, searchMessagesOutput, error) {
		filter, err := mcpMessageFilter(in)
		if err != nil {
			return nil, searchMessagesOutput{}, err
		}
		sortMode := strings.TrimSpace(in.Sort)
		switch sortMode {
		case "", "relevance":
			sortMode = "relevance"
		case "received_at":
		default:
			return nil, searchMessagesOutput{}, errors.New("sort must be relevance or received_at")
		}
		if strings.TrimSpace(in.Sender) != "" {
			filter.Query = strings.TrimSpace(filter.Query + " " + strings.ToLower(strings.TrimSpace(in.Sender)))
		}
		query := searchidx.Query{
			Text:      strings.TrimSpace(in.Query),
			Mailbox:   filter.Mailbox,
			Domain:    filter.Domain,
			Recipient: strings.ToLower(strings.TrimSpace(in.Recipient)),
			Sender:    strings.ToLower(strings.TrimSpace(in.Sender)),
			Action:    filter.Action,
			Port:      filter.Port,
			Since:     filter.Since,
			Until:     filter.Until,
			Limit:     filter.Limit,
			Offset:    filter.Offset,
			Sort:      sortMode,
		}
		messages, total, highlights, backend, err := s.performSearch(ctx, query, filter)
		if err != nil {
			return nil, searchMessagesOutput{}, err
		}
		out := searchMessagesOutput{
			Messages:   make([]mcpMessageSummary, 0, len(messages)),
			Count:      total,
			Highlights: highlights,
			Backend:    backend,
		}
		for _, message := range messages {
			out.Messages = append(out.Messages, mcpMessageFrom(message))
		}
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_events",
		Description: "List audit events. Use severity=error or type=message.received to narrow the stream.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listEventsInput) (*mcp.CallToolResult, listEventsOutput, error) {
		since, err := parseMCPTime(in.Since, "since")
		if err != nil {
			return nil, listEventsOutput{}, err
		}
		limit, offset := mcpPage(in.Limit, in.Offset)
		filter := models.EventFilter{
			Type:     strings.TrimSpace(in.Type),
			Severity: strings.TrimSpace(in.Severity),
			Query:    strings.TrimSpace(in.Query),
			Since:    since,
			Limit:    limit,
			Offset:   offset,
		}
		evs, err := s.store.ListEvents(ctx, filter)
		if err != nil {
			return nil, listEventsOutput{}, err
		}
		total, err := s.store.CountEvents(ctx, filter)
		if err != nil {
			return nil, listEventsOutput{}, err
		}
		out := listEventsOutput{Events: make([]mcpEvent, 0, len(evs)), Count: total}
		for _, event := range evs {
			out.Events = append(out.Events, mcpEventFrom(event))
		}
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_policies",
		Description: "List retention policies in evaluation order.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ listPoliciesInput) (*mcp.CallToolResult, listPoliciesOutput, error) {
		policies, err := s.store.ListPolicies(ctx)
		if err != nil {
			return nil, listPoliciesOutput{}, err
		}
		out := listPoliciesOutput{Policies: make([]mcpPolicy, 0, len(policies)), Count: len(policies)}
		for _, policy := range policies {
			out.Policies = append(out.Policies, mcpPolicyFrom(policy))
		}
		return nil, out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_stats",
		Description: "Get capture counts, policy count, event count, version, and uptime.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ getStatsInput) (*mcp.CallToolResult, getStatsOutput, error) {
		stats, err := s.gatherStats(ctx)
		if err != nil {
			return nil, getStatsOutput{}, err
		}
		return nil, getStatsOutput{
			Messages:    stats.TotalMessages,
			Messages24h: stats.Last24h,
			Policies:    stats.Policies,
			Events:      stats.Events,
			Version:     s.version,
			Uptime:      time.Since(s.startedAt).Round(time.Second).String(),
		}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_message",
		Description: "Delete one captured message. This is permanent and emits an audit event.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deleteMessageInput) (*mcp.CallToolResult, deleteMessageOutput, error) {
		message, err := s.store.GetMessage(ctx, in.ID)
		if err != nil {
			return nil, deleteMessageOutput{}, errors.New("message not found")
		}
		if err := s.store.DeleteMessage(ctx, message.ID); err != nil {
			return nil, deleteMessageOutput{}, err
		}
		s.bus.Emit(events.New(events.TypeMessageDeleted, events.SeverityInfo, map[string]any{
			"message_id": message.ID,
			"recipient":  message.Recipient,
			"mailbox":    message.Mailbox,
			"domain":     message.Domain,
			"local_port": message.LocalPort,
			"remote_ip":  message.RemoteIP,
			"subject":    message.Subject,
			"action":     message.Action,
			"reason":     "mcp deletion",
			"username":   "mcp",
		}))
		return nil, deleteMessageOutput{Deleted: true, ID: message.ID}, nil
	})
}

func parseMCPTime(value, field string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, errors.New(field + " must be an RFC 3339 timestamp")
	}
	return parsed.UTC(), nil
}

func mcpPage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func mcpMessageFilter(in searchMessagesInput) (models.MessageFilter, error) {
	since, err := parseMCPTime(in.Since, "since")
	if err != nil {
		return models.MessageFilter{}, err
	}
	until, err := parseMCPTime(in.Until, "until")
	if err != nil {
		return models.MessageFilter{}, err
	}
	limit, offset := mcpPage(in.Limit, in.Offset)
	filter := models.MessageFilter{
		Query:   strings.TrimSpace(in.Query),
		Domain:  strings.ToLower(strings.TrimSpace(in.Domain)),
		Mailbox: strings.ToLower(strings.TrimSpace(in.Mailbox)),
		Action:  strings.TrimSpace(in.Action),
		Port:    in.Port,
		Since:   since,
		Until:   until,
		Limit:   limit,
		Offset:  offset,
	}
	if strings.TrimSpace(in.Recipient) != "" {
		mailboxName, domain, err := mailbox.ParseAddress(in.Recipient)
		if err != nil {
			return models.MessageFilter{}, err
		}
		filter.Mailbox = mailboxName
		filter.Domain = domain
	}
	return filter, nil
}
