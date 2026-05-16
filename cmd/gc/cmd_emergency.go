package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/emergency"
	"github.com/gastownhall/gascity/internal/osnotify"
	"github.com/spf13/cobra"
)

const emergencyControllerCommandPrefix = "emergency:"

type emergencySendOptions struct {
	severity string
	ref      string
	actor    string
	metadata []string
	notify   bool
	quiet    bool
	message  string
	bodyFile string
}

type emergencyListOptions struct {
	all        bool
	limit      int
	format     string
	since      string
	severities []string
	actor      string
}

func newEmergencyCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "emergency",
		Short: "Send dolt-independent emergency signals",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(stderr, "gc emergency: missing subcommand (send)") //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stderr, "gc emergency: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
			}
			return errExit
		},
	}
	cmd.AddCommand(newEmergencySendCmd(stdout, stderr))
	cmd.AddCommand(newEmergencyListCmd(stdout, stderr))
	cmd.AddCommand(newEmergencyShowCmd(stdout, stderr))
	cmd.AddCommand(newEmergencyAckCmd(stdout, stderr))
	return cmd
}

func newEmergencySendCmd(stdout, stderr io.Writer) *cobra.Command {
	opts := emergencySendOptions{severity: emergency.SeverityError}
	cmd := &cobra.Command{
		Use:   "send [flags] [<message>]",
		Short: "Send a dolt-independent emergency signal",
		Long: `Send a dolt-independent emergency signal.

Use this when normal reporting paths such as bd update or gc mail send
cannot be trusted. The signal is written to a filesystem spool and then
best-effort forwarded to events.jsonl, the controller socket, and the host
notification system.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return exitForCode(cmdEmergencySend(cmd, args, opts, stdout, stderr))
		},
	}
	cmd.Flags().StringVarP(&opts.severity, "severity", "s", emergency.SeverityError, "info|warn|error|critical")
	cmd.Flags().StringVar(&opts.ref, "ref", "", "related bead id")
	cmd.Flags().StringVar(&opts.actor, "actor", "", "actor name (default: $GC_ALIAS, $GC_AGENT, $GC_SESSION_ID, $BEADS_ACTOR, human)")
	cmd.Flags().StringArrayVar(&opts.metadata, "metadata", nil, "metadata key=value (repeatable)")
	cmd.Flags().BoolVar(&opts.notify, "notify", false, "force OS notification regardless of severity")
	cmd.Flags().BoolVar(&opts.quiet, "quiet", false, "suppress OS notification regardless of severity")
	cmd.Flags().StringVar(&opts.message, "message", "", "message body (alternative to positional)")
	cmd.Flags().StringVar(&opts.bodyFile, "body-file", "", "read message from file (\"-\" = stdin)")
	return cmd
}

func newEmergencyListCmd(stdout, stderr io.Writer) *cobra.Command {
	opts := emergencyListOptions{format: "table"}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List emergency signals",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return exitForCode(cmdEmergencyList(opts, stdout, stderr))
		},
	}
	cmd.Flags().BoolVar(&opts.all, "all", false, "include processed entries")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum entries (default 50, or 20 for hook-injection)")
	cmd.Flags().StringVar(&opts.format, "format", "table", "table|json|hook-injection")
	cmd.Flags().StringVar(&opts.since, "since", "", "only entries newer than duration ago (for example 24h)")
	cmd.Flags().StringArrayVar(&opts.severities, "severity", nil, "filter by severity (repeatable)")
	cmd.Flags().StringVar(&opts.actor, "actor", "", "filter by actor substring")
	return cmd
}

func newEmergencyShowCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show one emergency signal",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return exitForCode(cmdEmergencyShow(args[0], stdout, stderr))
		},
	}
	return cmd
}

func newEmergencyAckCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ack <id>",
		Short: "Acknowledge one emergency signal",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return exitForCode(cmdEmergencyAck(args[0], stdout, stderr))
		},
	}
	return cmd
}

func cmdEmergencySend(cmd *cobra.Command, args []string, opts emergencySendOptions, stdout, stderr io.Writer) int {
	if opts.notify && opts.quiet {
		fmt.Fprintln(stderr, "gc emergency send: --notify and --quiet are mutually exclusive") //nolint:errcheck // best-effort stderr
		return 2
	}
	message, err := resolveEmergencyMessage(args, opts.message, opts.bodyFile, cmd.InOrStdin())
	if err != nil {
		fmt.Fprintf(stderr, "gc emergency send: %v\n", err) //nolint:errcheck // best-effort stderr
		return 2
	}
	metadata, err := parseEmergencyMetadata(opts.metadata)
	if err != nil {
		fmt.Fprintf(stderr, "gc emergency send: %v\n", err) //nolint:errcheck // best-effort stderr
		return 2
	}
	ref := strings.TrimSpace(opts.ref)
	if ref != "" && !isBeadIDCandidate(ref) {
		fmt.Fprintf(stderr, "gc emergency send: --ref %q malformed; spooled without ref\n", ref) //nolint:errcheck // best-effort stderr
		ref = ""
	}
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc emergency send: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	actor := strings.TrimSpace(opts.actor)
	if actor == "" {
		actor = eventActor()
	}
	cwd, _ := os.Getwd()
	hostname, _ := os.Hostname()
	rec, err := emergency.NewRecord(emergency.RecordOptions{
		Severity:   opts.severity,
		Actor:      actor,
		Message:    message,
		RefBead:    ref,
		SourcePath: cwd,
		SourcePID:  os.Getpid(),
		Hostname:   hostname,
		Metadata:   metadata,
	})
	if err != nil {
		fmt.Fprintf(stderr, "gc emergency send: %v\n", err) //nolint:errcheck // best-effort stderr
		if emergencyInputError(err) {
			return 2
		}
		return 1
	}
	if _, err := emergency.WriteSpool(cityPath, rec); err != nil {
		fmt.Fprintf(stderr, "gc emergency send: spool write failed: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	fmt.Fprintf(stderr, "gc emergency send: spooled (%s)\n", rec.ID) //nolint:errcheck // best-effort stderr
	if err := emergency.RecordSignaledToCityLog(cityPath, rec, stderr); err != nil {
		fmt.Fprintf(stderr, "gc emergency send: events recorder error: %v\n", err) //nolint:errcheck // best-effort stderr
	} else {
		fmt.Fprintln(stderr, "gc emergency send: events.jsonl recorded") //nolint:errcheck // best-effort stderr
	}
	if err := sendEmergencyToController(cityPath, rec, time.Second); err != nil {
		fmt.Fprintln(stderr, "gc emergency send: controller unreachable, spool only") //nolint:errcheck // best-effort stderr
	} else {
		fmt.Fprintln(stderr, "gc emergency send: controller socket ok") //nolint:errcheck // best-effort stderr
	}
	maybeNotifyEmergency(cityPath, rec, opts, stderr)
	fmt.Fprintln(stdout, rec.ID) //nolint:errcheck // best-effort stdout
	return 0
}

func cmdEmergencyList(opts emergencyListOptions, stdout, stderr io.Writer) int {
	format := strings.TrimSpace(opts.format)
	switch format {
	case "table", "json", "hook-injection":
	default:
		fmt.Fprintf(stderr, "gc emergency list: invalid --format %q\n", opts.format) //nolint:errcheck // best-effort stderr
		return 2
	}
	limit := opts.limit
	if limit == 0 {
		limit = 50
		if format == "hook-injection" {
			limit = 20
		}
	}
	if limit < 0 {
		fmt.Fprintln(stderr, "gc emergency list: --limit must be non-negative") //nolint:errcheck // best-effort stderr
		return 2
	}
	now := time.Now().UTC()
	var since time.Time
	if strings.TrimSpace(opts.since) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(opts.since))
		if err != nil || d < 0 {
			fmt.Fprintf(stderr, "gc emergency list: invalid --since %q\n", opts.since) //nolint:errcheck // best-effort stderr
			return 2
		}
		since = now.Add(-d)
	}
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc emergency list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	result, err := emergency.ListRecords(cityPath, emergency.ListOptions{
		IncludeAcked:   opts.all,
		Since:          since,
		Severities:     opts.severities,
		ActorSubstring: opts.actor,
		Limit:          limit,
		Now:            now,
	})
	if err != nil {
		fmt.Fprintf(stderr, "gc emergency list: %v\n", err) //nolint:errcheck // best-effort stderr
		if strings.Contains(err.Error(), "severity") {
			return 2
		}
		return 1
	}
	switch format {
	case "json":
		for _, entry := range result.Entries {
			data, err := json.Marshal(entry.Record)
			if err != nil {
				fmt.Fprintf(stderr, "gc emergency list: encoding record %q: %v\n", entry.Record.ID, err) //nolint:errcheck
				return 1
			}
			fmt.Fprintln(stdout, string(data)) //nolint:errcheck // best-effort stdout
		}
	case "hook-injection":
		fmt.Fprint(stdout, emergency.RenderHookInjection(result, now)) //nolint:errcheck // best-effort stdout
	default:
		writeEmergencyTable(stdout, result, now, opts.all)
	}
	return 0
}

func cmdEmergencyShow(id string, stdout, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc emergency show: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	entry, err := emergency.ShowRecord(cityPath, id)
	if err != nil {
		if errors.Is(err, emergency.ErrInvalidID) {
			fmt.Fprintf(stderr, "gc emergency show: invalid id format: %q\n", id) //nolint:errcheck // best-effort stderr
			return 2
		}
		if errors.Is(err, emergency.ErrNotFound) {
			fmt.Fprintf(stderr, "gc emergency show: no such emergency: %q\n", id) //nolint:errcheck // best-effort stderr
			return 2
		}
		fmt.Fprintf(stderr, "gc emergency show: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	data, err := json.MarshalIndent(entry.Record, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "gc emergency show: encoding record %q: %v\n", entry.Record.ID, err) //nolint:errcheck
		return 1
	}
	fmt.Fprintln(stdout, string(data)) //nolint:errcheck // best-effort stdout
	return 0
}

func cmdEmergencyAck(id string, stdout, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc emergency ack: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	result, err := emergency.AckRecord(cityPath, id, time.Now().UTC())
	if err != nil {
		if errors.Is(err, emergency.ErrInvalidID) {
			fmt.Fprintf(stderr, "gc emergency ack: invalid id format: %q\n", id) //nolint:errcheck // best-effort stderr
			return 2
		}
		if errors.Is(err, emergency.ErrNotFound) {
			fmt.Fprintf(stderr, "gc emergency ack: no such emergency: %q\n", id) //nolint:errcheck // best-effort stderr
			return 2
		}
		fmt.Fprintf(stderr, "gc emergency ack: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if result.AlreadyAcked {
		if !result.Entry.AckedAt.IsZero() {
			fmt.Fprintf(stdout, "already acked: %s (acked %s ago)\n", result.Entry.Record.ID, emergency.FormatAge(time.Now().UTC(), result.Entry.AckedAt)) //nolint:errcheck
		} else {
			fmt.Fprintf(stdout, "already acked: %s\n", result.Entry.Record.ID) //nolint:errcheck
		}
		return 0
	}
	actor := eventActor()
	if err := emergency.RecordAckedToCityLog(cityPath, result.Entry.Record, actor, stderr); err != nil {
		fmt.Fprintf(stderr, "gc emergency ack: events recorder error: %v\n", err) //nolint:errcheck // best-effort stderr
	}
	fmt.Fprintf(stdout, "acked: %s\n", result.Entry.Record.ID) //nolint:errcheck // best-effort stdout
	return 0
}

func writeEmergencyTable(stdout io.Writer, result emergency.ListResult, now time.Time, all bool) {
	if len(result.Entries) == 0 {
		if all {
			fmt.Fprintf(stdout, "0 entries. 0 unacked, 0 acked.\n") //nolint:errcheck // best-effort stdout
			return
		}
		fmt.Fprintf(stdout, "0 unacked.\n") //nolint:errcheck // best-effort stdout
		return
	}
	if all {
		fmt.Fprintf(stdout, "%-31s %-8s %-5s %-6s %-26s %-56s %s\n", "ID", "SEVERITY", "AGE", "STATUS", "ACTOR", "MESSAGE", "REF") //nolint:errcheck
	} else {
		fmt.Fprintf(stdout, "%-31s %-8s %-5s %-26s %-56s %s\n", "ID", "SEVERITY", "AGE", "ACTOR", "MESSAGE", "REF") //nolint:errcheck
	}
	for _, entry := range result.Entries {
		rec := entry.Record
		ref := strings.TrimSpace(rec.RefBead)
		if ref == "" {
			ref = "-"
		}
		message := truncateEmergencyTable(rec.Message, 55)
		if all {
			fmt.Fprintf(stdout, "%-31s %-8s %-5s %-6s %-26s %-56s %s\n", rec.ID, rec.Severity, emergency.FormatAge(now, rec.CreatedAt), entry.Status, rec.Actor, message, ref) //nolint:errcheck
		} else {
			fmt.Fprintf(stdout, "%-31s %-8s %-5s %-26s %-56s %s\n", rec.ID, rec.Severity, emergency.FormatAge(now, rec.CreatedAt), rec.Actor, message, ref) //nolint:errcheck
		}
	}
	if all {
		fmt.Fprintf(stdout, "\n%d entries. %d unacked, %d acked.\n", result.Total, result.Open, result.Acked) //nolint:errcheck
		return
	}
	fmt.Fprintf(stdout, "\n%d unacked. Run `gc emergency ack <ID>` to clear.\n", result.Open) //nolint:errcheck
}

func resolveEmergencyMessage(args []string, messageFlag, bodyFile string, stdin io.Reader) (string, error) {
	forms := 0
	if len(args) > 0 {
		forms++
	}
	if strings.TrimSpace(messageFlag) != "" {
		forms++
	}
	if strings.TrimSpace(bodyFile) != "" {
		forms++
	}
	if forms > 1 {
		return "", fmt.Errorf("--message, --body-file, and a positional message are mutually exclusive")
	}
	switch {
	case len(args) > 0:
		return strings.TrimSpace(strings.Join(args, " ")), nil
	case strings.TrimSpace(messageFlag) != "":
		return strings.TrimSpace(messageFlag), nil
	case strings.TrimSpace(bodyFile) == "-":
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("reading --body-file -: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	case strings.TrimSpace(bodyFile) != "":
		data, err := os.ReadFile(filepath.Clean(bodyFile))
		if err != nil {
			return "", fmt.Errorf("reading --body-file %q: %w", bodyFile, err)
		}
		return strings.TrimSpace(string(data)), nil
	default:
		return "", fmt.Errorf("message is required")
	}
}

func parseEmergencyMetadata(entries []string) (map[string]string, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(entries))
	for _, raw := range entries {
		key, value, ok := strings.Cut(raw, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("--metadata must be key=value")
		}
		if emergencyReservedMetadataKey(key) {
			return nil, fmt.Errorf("--metadata key %q is reserved", key)
		}
		out[key] = strings.TrimSpace(value)
	}
	return out, nil
}

func emergencyReservedMetadataKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "id", "severity", "actor", "message", "ref_bead", "source_path", "source_pid", "hostname", "created_at", "metadata":
		return true
	default:
		return false
	}
}

func emergencyInputError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "severity") ||
		strings.Contains(msg, "message is required") ||
		strings.Contains(msg, "4 KiB")
}

func sendEmergencyToController(cityPath string, rec emergency.Record, timeout time.Duration) error {
	payload, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("encoding controller emergency request: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", controllerSocketPath(cityPath))
	if err != nil {
		return err
	}
	defer conn.Close() //nolint:errcheck // best-effort cleanup
	deadline := time.Now().Add(timeout)
	_ = conn.SetDeadline(deadline)
	if _, err := conn.Write(append([]byte(emergencyControllerCommandPrefix), append(payload, '\n')...)); err != nil {
		return err
	}
	buf := make([]byte, 16)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}
	if string(buf[:n]) != "ok\n" {
		return fmt.Errorf("unexpected controller response %q", string(buf[:n]))
	}
	return nil
}

func maybeNotifyEmergency(cityPath string, rec emergency.Record, opts emergencySendOptions, stderr io.Writer) {
	if opts.quiet {
		return
	}
	if !opts.notify && rec.Severity != emergency.SeverityCritical {
		return
	}
	dedupe, err := emergency.MarkNotifyDedupe(
		cityPath,
		emergency.NotifyDedupeKey(rec.Severity, rec.Message),
		time.Now(),
		5*time.Minute,
	)
	if err != nil {
		fmt.Fprintf(stderr, "gc emergency send: notify dedupe error: %v, spool only\n", err) //nolint:errcheck // best-effort stderr
		return
	}
	if !dedupe.Fire {
		fmt.Fprintf(stderr, "gc emergency send: notify dedupe (recent same-severity message, key %s, fired %s ago)\n", dedupe.KeyPrefix, dedupe.Age.Round(time.Second)) //nolint:errcheck // best-effort stderr
		return
	}
	result := osnotify.Notify(context.Background(), osnotify.Notification{
		Severity: rec.Severity,
		Actor:    rec.Actor,
		Message:  rec.Message,
		RefBead:  rec.RefBead,
	}, osnotify.Dependencies{})
	if result.Err != nil {
		fmt.Fprintf(stderr, "gc emergency send: %s error: %v, spool only\n", result.Backend, result.Err) //nolint:errcheck // best-effort stderr
		return
	}
	if !result.Fired {
		fmt.Fprintf(stderr, "gc emergency send: %s not on PATH, spool only\n", result.Backend) //nolint:errcheck // best-effort stderr
		return
	}
	fmt.Fprintf(stderr, "gc emergency send: %s fired\n", result.Backend) //nolint:errcheck // best-effort stderr
}

func truncateEmergencyTable(message string, limit int) string {
	msg := strings.Join(strings.Fields(message), " ")
	if limit <= 0 || len(msg) <= limit {
		return msg
	}
	if limit <= 3 {
		return msg[:limit]
	}
	return msg[:limit-3] + "..."
}
