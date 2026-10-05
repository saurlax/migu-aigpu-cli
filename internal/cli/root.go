package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/saurlax/migu-aigpu-cli/internal/api"
	"github.com/saurlax/migu-aigpu-cli/internal/auth"
	"github.com/spf13/cobra"
)

var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type options struct {
	json, dry bool
	team      string
	timeout   time.Duration
}

func New(version string) *cobra.Command {
	o := &options{}
	root := &cobra.Command{Use: "migu", Short: "Unofficial Migu AIGPU CLI", Version: version, SilenceErrors: true, SilenceUsage: true, Args: cobra.NoArgs}
	root.PersistentFlags().BoolVar(&o.json, "json", false, "Print sanitized JSON for automation")
	root.PersistentFlags().BoolVar(&o.dry, "dry-run", false, "Preview API request without authentication or network access")
	root.PersistentFlags().StringVar(&o.team, "team", "", "Explicit platform team ID")
	root.PersistentFlags().DurationVar(&o.timeout, "timeout", 60*time.Second, "HTTP request timeout")
	root.AddCommand(authCommands(o), instanceCommands(o), imageCommands(o), datasetCommands(o), storageCommands(o))
	return root
}

func client(o *options) (*api.Client, error) {
	if o.timeout <= 0 {
		return nil, errors.New("timeout must be positive")
	}
	s, err := auth.DefaultStore()
	if err != nil {
		return nil, err
	}
	return api.New(s, o.timeout, o.team), nil
}

func emit(cmd *cobra.Command, v any) error {
	e := json.NewEncoder(cmd.OutOrStdout())
	e.SetIndent("", "  ")
	e.SetEscapeHTML(false)
	// Normalize typed values before recursively masking fields.
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var plain any
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.UseNumber()
	if err = d.Decode(&plain); err != nil {
		return err
	}
	return e.Encode(api.Redact(plain))
}

func run(cmd *cobra.Command, o *options, method, path string, body any, readOnly bool, execute bool) (map[string]any, error) {
	if o.timeout <= 0 {
		return nil, errors.New("timeout must be positive")
	}
	if o.dry || (!readOnly && !execute) {
		return nil, emit(cmd, map[string]any{"dry_run": true, "method": method, "url": api.Origin + path, "body": body})
	}
	c, err := client(o)
	if err != nil {
		return nil, err
	}
	return c.Call(cmd.Context(), method, path, body, readOnly)
}

func authCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "auth", Short: "Import, inspect, and renew an encrypted session", Args: cobra.NoArgs}
	status := &cobra.Command{Use: "status", Short: "Inspect local expiry without refreshing or network access", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		s, err := auth.DefaultStore()
		if err != nil {
			return err
		}
		release, err := s.Lock(cmd.Context())
		if err != nil {
			return err
		}
		defer release()
		state, err := s.Load()
		if err != nil {
			return emit(cmd, map[string]any{"imported": false, "store": s.Dir, "message": err.Error()})
		}
		return emit(cmd, metadata(state, false))
	}}
	for _, action := range []string{"ensure", "refresh"} {
		group.AddCommand(&cobra.Command{Use: action, Short: map[string]string{"ensure": "Renew only when within one hour of expiry", "refresh": "Force one refresh now"}[action], Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
			if o.dry {
				return emit(cmd, map[string]any{"dry_run": true, "action": action})
			}
			c, err := client(o)
			if err != nil {
				return err
			}
			s, renewed, err := c.Ensure(cmd.Context(), action == "refresh")
			if err != nil {
				return err
			}
			return emit(cmd, metadata(s, renewed))
		}})
	}
	var duration time.Duration
	capture := &cobra.Command{Use: "capture", Short: "Import a logged-in browser session via a one-shot local bridge", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if duration <= 0 || duration > 15*time.Minute {
			return errors.New("capture duration must be between 0 and 15 minutes")
		}
		if o.dry {
			return emit(cmd, map[string]any{"dry_run": true, "action": "capture"})
		}
		s, err := auth.DefaultStore()
		if err != nil {
			return err
		}
		return auth.Capture(cmd.Context(), s, cmd.OutOrStdout(), duration)
	}}
	capture.Flags().DurationVar(&duration, "wait", 5*time.Minute, "Foreground capture deadline")
	legacy := &cobra.Command{Use: "import-legacy", Short: "Copy the Windows Python prototype session into this CLI's encrypted store", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if o.dry {
			return emit(cmd, map[string]any{"dry_run": true, "action": "import-legacy"})
		}
		s, err := auth.DefaultStore()
		if err != nil {
			return err
		}
		release, err := s.Lock(cmd.Context())
		if err != nil {
			return err
		}
		defer release()
		// Never overwrite an already imported Go session with an older credential.
		if _, err = s.Load(); err == nil {
			return errors.New("session already exists; use auth capture to replace it")
		}
		if err = auth.ImportLegacy(s); err != nil {
			return err
		}
		return emit(cmd, map[string]any{"imported": true})
	}}
	logout := &cobra.Command{Use: "logout", Short: "Delete this CLI's local session (does not revoke browser login)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if o.dry {
			return emit(cmd, map[string]any{"dry_run": true, "action": "logout"})
		}
		s, err := auth.DefaultStore()
		if err != nil {
			return err
		}
		release, err := s.Lock(cmd.Context())
		if err != nil {
			return err
		}
		defer release()
		if err = s.Delete(); err != nil {
			return errors.New("cannot remove local session")
		}
		return emit(cmd, map[string]any{"removed": true})
	}}
	group.AddCommand(status, capture, legacy, logout)
	return group
}

func metadata(s auth.Session, renewed bool) map[string]any {
	return map[string]any{"imported": true, "renewed": renewed, "refresh_token_present": s.Refresh != "", "renewal_due": s.Due(time.Now()), "access_seconds_left": int64(s.ExpiresAt) - time.Now().Unix(), "expires_at": time.Unix(int64(s.ExpiresAt), 0).UTC().Format(time.RFC3339), "last_refresh": s.LastRefresh}
}

func idArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.ExactArgs(1)(cmd, args); err != nil {
		return err
	}
	if !uuid.MatchString(args[0]) {
		return errors.New("instance/image ID must be a UUID")
	}
	return nil
}

func pagination(page, size int) error {
	if page < 1 || size < 1 || size > 100 {
		return errors.New("page must be positive; page-size must be 1..100")
	}
	return nil
}

func instanceCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "instance", Aliases: []string{"instances"}, Short: "List, inspect, or power an existing instance", Args: cobra.NoArgs}
	var page, size int
	var idc, status string
	list := &cobra.Command{Use: "list", Short: "List instances", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := pagination(page, size); err != nil {
			return err
		}
		body := map[string]any{"page": page, "pageSize": size}
		if idc != "" {
			body["idcId"] = idc
		}
		if status != "" {
			body["status"] = status
		}
		r, err := run(cmd, o, "POST", api.Instance+"/search", body, true, false)
		if err != nil || r == nil {
			return err
		}
		if o.json {
			return emit(cmd, r)
		}
		data, _ := r["data"].(map[string]any)
		rows, _ := data["instances"].([]any)
		if rows == nil {
			return emit(cmd, r)
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tSTATUS\tIDC")
		for _, row := range rows {
			v, _ := api.Redact(row).(map[string]any)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", cell(v, "id", "uuid", "instanceUuid"), cell(v, "name", "instanceName"), cell(v, "status"), cell(v, "idcId"))
		}
		return w.Flush()
	}}
	list.Flags().IntVar(&page, "page", 1, "Page number")
	list.Flags().IntVar(&size, "page-size", 20, "Items per page (1..100)")
	list.Flags().StringVar(&idc, "idc", "", "Data center ID")
	list.Flags().StringVar(&status, "status", "", "Platform status filter")
	group.AddCommand(list)
	for _, action := range []string{"get", "status", "metrics", "start", "stop"} {
		execute := false
		mode := "GPU"
		command := &cobra.Command{Use: action + " INSTANCE_ID", Short: map[string]string{"get": "Inspect a sanitized instance snapshot", "status": "Read lifecycle status", "metrics": "Query current metrics", "start": "Power on (preview unless --execute)", "stop": "Power off (preview unless --execute)"}[action], Args: idArgs, RunE: func(cmd *cobra.Command, args []string) error {
			path := api.Instance + "/" + args[0]
			method := "GET"
			var body any
			readOnly := action != "start" && action != "stop"
			if action == "metrics" {
				path = api.Instance + "/metric/batchQryNowMetricsByInstanceUuidList"
				method = "POST"
				body = []string{args[0]}
			}
			if !readOnly {
				path += "/" + action
				method = "PUT"
				body = map[string]any{}
				if action == "start" {
					if mode != "GPU" && mode != "CPU" {
						return errors.New("mode must be GPU or CPU")
					}
					body = map[string]any{"startMode": mode}
				}
			}
			r, err := run(cmd, o, method, path, body, readOnly, execute)
			if err != nil || r == nil {
				return err
			}
			if !readOnly {
				// A successful power request is an acknowledgement, not a final status.
				c, err := client(o)
				if err != nil {
					return err
				}
				observed, observeErr := c.Call(cmd.Context(), "GET", api.Instance+"/"+args[0], nil, true)
				v := map[string]any{"acknowledgement": r, "observed_after_request": observed}
				if observeErr != nil {
					v["observation_error"] = observeErr.Error()
				}
				return emit(cmd, v)
			}
			if action == "status" && !o.json {
				data, _ := r["data"].(map[string]any)
				if value, ok := data["status"]; ok {
					_, err = fmt.Fprintln(cmd.OutOrStdout(), api.Redact(value))
					return err
				}
			}
			return emit(cmd, r)
		}}
		if action == "start" || action == "stop" {
			command.Flags().BoolVar(&execute, "execute", false, "Send the power request (can affect billing)")
		}
		if action == "start" {
			command.Flags().StringVar(&mode, "mode", "GPU", "Start mode: GPU or CPU")
		}
		group.AddCommand(command)
	}
	return group
}

// Escape control characters so server-supplied names cannot inject terminal commands.
func cell(row map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := row[k]; ok && v != nil {
			return strings.Map(func(r rune) rune {
				if r < 32 || r == 127 {
					return ' '
				}
				return r
			}, fmt.Sprint(v))
		}
	}
	return "-"
}

func imageCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "image", Aliases: []string{"mirror", "mirrors"}, Short: "Query available public/private model images", Args: cobra.NoArgs}
	var private bool
	var idc string
	list := &cobra.Command{Use: "list", Short: "List available image versions (platform hierarchy retained)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		source := "public"
		if private {
			source = "private"
		}
		body := map[string]any{"source": source, "status": "available"}
		if idc != "" {
			body["idcId"] = idc
		}
		r, err := run(cmd, o, "POST", "/v1/cloud/traininfer/mirrorversion/list", body, true, false)
		if err != nil || r == nil {
			return err
		}
		return emit(cmd, r)
	}}
	list.Flags().BoolVar(&private, "private", false, "Query private images")
	list.Flags().StringVar(&idc, "idc", "", "Data center ID")
	group.AddCommand(list)
	return group
}

func datasetCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "dataset", Aliases: []string{"datasets"}, Short: "Query the dataset warehouse", Args: cobra.NoArgs}
	var page, size int
	list := &cobra.Command{Use: "list", Short: "List datasets", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := pagination(page, size); err != nil {
			return err
		}
		q := url.Values{"pageNum": {fmt.Sprint(page)}, "pageSize": {fmt.Sprint(size)}}
		r, err := run(cmd, o, "GET", "/v1/cloud/traininfer/dataset/list?"+q.Encode(), nil, true, false)
		if err != nil || r == nil {
			return err
		}
		return emit(cmd, r)
	}}
	list.Flags().IntVar(&page, "page", 1, "Page number")
	list.Flags().IntVar(&size, "page-size", 20, "Items per page (1..100)")
	group.AddCommand(list)
	return group
}

func storageCommands(o *options) *cobra.Command {
	group := &cobra.Command{Use: "storage", Short: "Query personal network storage", Args: cobra.NoArgs}
	var idc string
	list := &cobra.Command{Use: "list", Short: "List storage for a data center", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		r, err := run(cmd, o, "POST", "/v1/cloud/traininfer/filestorage/list", map[string]any{"idcId": idc, "queryTeam": false}, true, false)
		if err != nil || r == nil {
			return err
		}
		return emit(cmd, r)
	}}
	list.Flags().StringVar(&idc, "idc", "", "Data center ID (required)")
	_ = list.MarkFlagRequired("idc")
	group.AddCommand(list)
	return group
}
