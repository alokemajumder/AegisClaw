package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	cliVersion   = "1.0.0"
	cliBuildDate = "2026-03-04"
	tokenFile    = ".aegisclaw/token"
)

// apiClient wraps HTTP calls to the AegisClaw API gateway.
type apiClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func newAPIClient() *apiClient {
	base := os.Getenv("AEGISCLAW_API_URL")
	if base == "" {
		base = "http://localhost:8080"
	}
	base = strings.TrimRight(base, "/")

	c := &apiClient{
		baseURL: base,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	c.token = c.loadToken()
	return c
}

// tokenPath returns the full path to the token file.
func tokenPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, tokenFile)
}

// loadToken reads the stored JWT token from disk.
func (c *apiClient) loadToken() string {
	p := tokenPath()
	if p == "" {
		return ""
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// saveToken persists the JWT token to disk.
func (c *apiClient) saveToken(token string) error {
	p := tokenPath()
	if p == "" {
		return fmt.Errorf("unable to determine home directory")
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	return os.WriteFile(p, []byte(token+"\n"), 0600)
}

// request performs an HTTP request and returns the parsed JSON response.
func (c *apiClient) request(method, path string, body any) (json.RawMessage, error) {
	u := c.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, u, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	// Try to parse as API envelope
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Meta json.RawMessage `json:"meta"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		// Not a JSON response or unexpected format
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
		}
		return respBody, nil
	}

	if envelope.Error != nil {
		return nil, fmt.Errorf("[%s] %s", envelope.Error.Code, envelope.Error.Message)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	// If data is present, return the full envelope for context (meta, etc.)
	return respBody, nil
}

// get is a convenience wrapper for GET requests.
func (c *apiClient) get(path string) (json.RawMessage, error) {
	return c.request(http.MethodGet, path, nil)
}

// post is a convenience wrapper for POST requests.
func (c *apiClient) post(path string, body any) (json.RawMessage, error) {
	return c.request(http.MethodPost, path, body)
}

// delete is a convenience wrapper for DELETE requests.
func (c *apiClient) del(path string) (json.RawMessage, error) {
	return c.request(http.MethodDelete, path, nil)
}

// prettyPrint prints JSON with indentation.
func prettyPrint(data json.RawMessage) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		fmt.Fprintln(os.Stdout, string(data))
		return
	}
	fmt.Fprintln(os.Stdout, buf.String())
}

// fatal prints an error and exits.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

// requireArg checks that at least n positional args exist after the subcommand.
func requireArg(args []string, n int, usage string) {
	if len(args) < n {
		fatal("missing required argument\nUsage: %s", usage)
	}
}

// parseFlags is a simple flag parser that extracts --key value pairs from args.
// Returns a map of flags and a slice of positional arguments.
func parseFlags(args []string) (flags map[string]string, positional []string) {
	flags = make(map[string]string)
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") {
			key := strings.TrimPrefix(args[i], "--")
			if strings.Contains(key, "=") {
				parts := strings.SplitN(key, "=", 2)
				flags[parts[0]] = parts[1]
			} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				flags[key] = args[i+1]
				i++
			} else {
				// Boolean-style flag with no value
				flags[key] = "true"
			}
		} else {
			positional = append(positional, args[i])
		}
	}
	return flags, positional
}

// requireFlag returns the value for a flag or exits with an error.
func requireFlag(flags map[string]string, name, usage string) string {
	v, ok := flags[name]
	if !ok || v == "" {
		fatal("missing required flag: --%s\nUsage: %s", name, usage)
	}
	return v
}

// ---------- Commands ----------

func cmdLogin(c *apiClient, args []string) {
	flags, _ := parseFlags(args)
	email := requireFlag(flags, "email", "aegiscli login --email <email> --password <password>")
	password := requireFlag(flags, "password", "aegiscli login --email <email> --password <password>")

	body := map[string]string{
		"email":    email,
		"password": password,
	}

	resp, err := c.post("/api/v1/auth/login", body)
	if err != nil {
		fatal("login failed: %v", err)
	}

	// Extract access_token from data
	var envelope struct {
		Data struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			TokenType    string `json:"token_type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &envelope); err != nil {
		fatal("parsing login response: %v", err)
	}

	if envelope.Data.AccessToken == "" {
		fatal("no access token in response")
	}

	if err := c.saveToken(envelope.Data.AccessToken); err != nil {
		fatal("saving token: %v", err)
	}

	fmt.Println("Login successful. Token stored at ~/" + tokenFile)
}

func cmdAssetsList(c *apiClient, args []string) {
	flags, _ := parseFlags(args)
	q := url.Values{}
	if v, ok := flags["page"]; ok {
		q.Set("page", v)
	}
	if v, ok := flags["per-page"]; ok {
		q.Set("per_page", v)
	}

	path := "/api/v1/assets"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	resp, err := c.get(path)
	if err != nil {
		fatal("listing assets: %v", err)
	}
	prettyPrint(resp)
}

func cmdAssetsCreate(c *apiClient, args []string) {
	flags, _ := parseFlags(args)
	name := requireFlag(flags, "name", "aegiscli assets create --name <name> --type <type> --hostname <host>")
	assetType := requireFlag(flags, "type", "aegiscli assets create --name <name> --type <type> --hostname <host>")
	hostname := requireFlag(flags, "hostname", "aegiscli assets create --name <name> --type <type> --hostname <host>")

	body := map[string]any{
		"name":       name,
		"asset_type": assetType,
		"metadata":   map[string]string{"hostname": hostname},
	}

	resp, err := c.post("/api/v1/assets", body)
	if err != nil {
		fatal("creating asset: %v", err)
	}
	prettyPrint(resp)
}

func cmdAssetsDelete(c *apiClient, args []string) {
	_, positional := parseFlags(args)
	if len(positional) < 1 {
		fatal("missing asset ID\nUsage: aegiscli assets delete <id>")
	}
	id := positional[0]

	resp, err := c.del("/api/v1/assets/" + id)
	if err != nil {
		fatal("deleting asset: %v", err)
	}
	prettyPrint(resp)
}

func cmdEngagementsList(c *apiClient, args []string) {
	flags, _ := parseFlags(args)
	q := url.Values{}
	if v, ok := flags["page"]; ok {
		q.Set("page", v)
	}
	if v, ok := flags["per-page"]; ok {
		q.Set("per_page", v)
	}

	path := "/api/v1/engagements"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	resp, err := c.get(path)
	if err != nil {
		fatal("listing engagements: %v", err)
	}
	prettyPrint(resp)
}

func cmdEngagementsCreate(c *apiClient, args []string) {
	flags, _ := parseFlags(args)
	name := requireFlag(flags, "name", "aegiscli engagements create --name <name> --tiers 0,1")

	tiers := []int{0, 1}
	if v, ok := flags["tiers"]; ok {
		tiers = nil
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			n, err := strconv.Atoi(s)
			if err != nil {
				fatal("invalid tier value: %s", s)
			}
			tiers = append(tiers, n)
		}
	}

	body := map[string]any{
		"name":          name,
		"allowed_tiers": tiers,
	}

	resp, err := c.post("/api/v1/engagements", body)
	if err != nil {
		fatal("creating engagement: %v", err)
	}
	prettyPrint(resp)
}

func cmdEngagementsTrigger(c *apiClient, args []string) {
	_, positional := parseFlags(args)
	if len(positional) < 1 {
		fatal("missing engagement ID\nUsage: aegiscli engagements trigger <id>")
	}
	id := positional[0]

	resp, err := c.post("/api/v1/engagements/"+id+"/runs", nil)
	if err != nil {
		fatal("triggering engagement: %v", err)
	}
	prettyPrint(resp)
}

func cmdRunsList(c *apiClient, args []string) {
	flags, _ := parseFlags(args)
	q := url.Values{}
	if v, ok := flags["status"]; ok {
		q.Set("status", v)
	}
	if v, ok := flags["page"]; ok {
		q.Set("page", v)
	}
	if v, ok := flags["per-page"]; ok {
		q.Set("per_page", v)
	}

	path := "/api/v1/runs"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	resp, err := c.get(path)
	if err != nil {
		fatal("listing runs: %v", err)
	}
	prettyPrint(resp)
}

func cmdRunsGet(c *apiClient, args []string) {
	_, positional := parseFlags(args)
	if len(positional) < 1 {
		fatal("missing run ID\nUsage: aegiscli runs get <id>")
	}
	id := positional[0]

	resp, err := c.get("/api/v1/runs/" + id)
	if err != nil {
		fatal("getting run: %v", err)
	}
	prettyPrint(resp)
}

func cmdRunsKill(c *apiClient, args []string) {
	_, positional := parseFlags(args)
	if len(positional) < 1 {
		fatal("missing run ID\nUsage: aegiscli runs kill <id>")
	}
	id := positional[0]

	resp, err := c.post("/api/v1/runs/"+id+"/kill", nil)
	if err != nil {
		fatal("killing run: %v", err)
	}
	prettyPrint(resp)
}

func cmdFindingsList(c *apiClient, args []string) {
	flags, _ := parseFlags(args)
	q := url.Values{}
	if v, ok := flags["severity"]; ok {
		q.Set("severity", v)
	}
	if v, ok := flags["status"]; ok {
		q.Set("status", v)
	}
	if v, ok := flags["page"]; ok {
		q.Set("page", v)
	}
	if v, ok := flags["per-page"]; ok {
		q.Set("per_page", v)
	}

	path := "/api/v1/findings"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	resp, err := c.get(path)
	if err != nil {
		fatal("listing findings: %v", err)
	}
	prettyPrint(resp)
}

func cmdFindingsGet(c *apiClient, args []string) {
	_, positional := parseFlags(args)
	if len(positional) < 1 {
		fatal("missing finding ID\nUsage: aegiscli findings get <id>")
	}
	id := positional[0]

	resp, err := c.get("/api/v1/findings/" + id)
	if err != nil {
		fatal("getting finding: %v", err)
	}
	prettyPrint(resp)
}

func cmdFindingsTicket(c *apiClient, args []string) {
	_, positional := parseFlags(args)
	if len(positional) < 1 {
		fatal("missing finding ID\nUsage: aegiscli findings ticket <id>")
	}
	id := positional[0]

	resp, err := c.post("/api/v1/findings/"+id+"/ticket", map[string]string{})
	if err != nil {
		fatal("creating ticket for finding: %v", err)
	}
	prettyPrint(resp)
}

func cmdReportsList(c *apiClient, args []string) {
	flags, _ := parseFlags(args)
	q := url.Values{}
	if v, ok := flags["page"]; ok {
		q.Set("page", v)
	}
	if v, ok := flags["per-page"]; ok {
		q.Set("per_page", v)
	}

	path := "/api/v1/reports"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	resp, err := c.get(path)
	if err != nil {
		fatal("listing reports: %v", err)
	}
	prettyPrint(resp)
}

func cmdReportsGenerate(c *apiClient, args []string) {
	flags, _ := parseFlags(args)
	title := requireFlag(flags, "title", "aegiscli reports generate --title <title> --type <executive|technical|coverage>")
	reportType := requireFlag(flags, "type", "aegiscli reports generate --title <title> --type <executive|technical|coverage>")

	validTypes := map[string]bool{"executive": true, "technical": true, "coverage": true}
	if !validTypes[reportType] {
		fatal("invalid report type: %s (must be executive, technical, or coverage)", reportType)
	}

	body := map[string]string{
		"title":       title,
		"report_type": reportType,
	}

	resp, err := c.post("/api/v1/reports/generate", body)
	if err != nil {
		fatal("generating report: %v", err)
	}
	prettyPrint(resp)
}

func cmdKillSwitchEngage(c *apiClient, args []string) {
	flags, _ := parseFlags(args)
	reason := flags["reason"]
	if reason == "" {
		reason = "Engaged via CLI"
	}

	body := map[string]any{
		"engaged": true,
		"reason":  reason,
	}

	resp, err := c.post("/api/v1/admin/system/kill-switch", body)
	if err != nil {
		fatal("engaging kill switch: %v", err)
	}
	prettyPrint(resp)
}

func cmdKillSwitchDisengage(c *apiClient, args []string) {
	flags, _ := parseFlags(args)
	reason := flags["reason"]
	if reason == "" {
		reason = "Disengaged via CLI"
	}

	body := map[string]any{
		"engaged": false,
		"reason":  reason,
	}

	resp, err := c.post("/api/v1/admin/system/kill-switch", body)
	if err != nil {
		fatal("disengaging kill switch: %v", err)
	}
	prettyPrint(resp)
}

func cmdKillSwitchStatus(c *apiClient, _ []string) {
	resp, err := c.get("/api/v1/admin/system/health")
	if err != nil {
		fatal("checking kill switch status: %v", err)
	}
	prettyPrint(resp)
}

func cmdHealth(c *apiClient, _ []string) {
	resp, err := c.get("/healthz")
	if err != nil {
		fatal("health check failed: %v", err)
	}
	prettyPrint(resp)
}

func cmdVersion(_ *apiClient, _ []string) {
	info := map[string]string{
		"version":    cliVersion,
		"build_date": cliBuildDate,
		"api_url":    os.Getenv("AEGISCLAW_API_URL"),
	}
	if info["api_url"] == "" {
		info["api_url"] = "http://localhost:8080"
	}
	data, _ := json.MarshalIndent(info, "", "  ")
	fmt.Println(string(data))
}

// ---------- Usage ----------

func printUsage() {
	usage := `AegisClaw CLI — Autonomous Security Validation Platform

Usage: aegiscli <command> [subcommand] [flags]

Authentication:
  login                  Authenticate and store token
    --email <email>      User email (required)
    --password <pass>    User password (required)

Assets:
  assets list            List assets
    [--page N]           Page number (default 1)
    [--per-page N]       Items per page (default 50)
  assets create          Create a new asset
    --name <name>        Asset name (required)
    --type <type>        Asset type: server, endpoint, etc. (required)
    --hostname <host>    Hostname (required)
  assets delete <id>     Delete an asset by ID

Engagements:
  engagements list       List engagements
  engagements create     Create a new engagement
    --name <name>        Engagement name (required)
    [--tiers 0,1]        Comma-separated tier list (default 0,1)
  engagements trigger <id>
                         Trigger a run for an engagement

Runs:
  runs list              List validation runs
    [--status <status>]  Filter by status
  runs get <id>          Get run details
  runs kill <id>         Kill a running validation

Findings:
  findings list          List findings
    [--severity <sev>]   Filter by severity
    [--status <status>]  Filter by status
  findings get <id>      Get finding details
  findings ticket <id>   Create a ticket for a finding

Reports:
  reports list           List reports
  reports generate       Generate a new report
    --title <title>      Report title (required)
    --type <type>        Type: executive, technical, coverage (required)

Kill Switch:
  kill-switch engage     Engage the kill switch (stops all runs)
    [--reason <reason>]  Reason for engaging
  kill-switch disengage  Disengage the kill switch
    [--reason <reason>]  Reason for disengaging
  kill-switch status     Check kill switch status

System:
  health                 Check API gateway health
  version                Print CLI version info

Environment:
  AEGISCLAW_API_URL      API gateway URL (default: http://localhost:8080)

Token Storage:
  ~/.aegisclaw/token     JWT token stored after login
`
	fmt.Print(usage)
}

// ---------- Main ----------

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	client := newAPIClient()

	// Determine the command and subcommand
	cmd := os.Args[1]
	subArgs := os.Args[2:]

	switch cmd {
	case "help", "--help", "-h":
		printUsage()
		os.Exit(0)

	case "login":
		cmdLogin(client, subArgs)

	case "assets":
		if len(subArgs) == 0 {
			fatal("missing subcommand for 'assets'\nUsage: aegiscli assets <list|create|delete>")
		}
		switch subArgs[0] {
		case "list":
			cmdAssetsList(client, subArgs[1:])
		case "create":
			cmdAssetsCreate(client, subArgs[1:])
		case "delete":
			cmdAssetsDelete(client, subArgs[1:])
		default:
			fatal("unknown assets subcommand: %s\nUsage: aegiscli assets <list|create|delete>", subArgs[0])
		}

	case "engagements":
		if len(subArgs) == 0 {
			fatal("missing subcommand for 'engagements'\nUsage: aegiscli engagements <list|create|trigger>")
		}
		switch subArgs[0] {
		case "list":
			cmdEngagementsList(client, subArgs[1:])
		case "create":
			cmdEngagementsCreate(client, subArgs[1:])
		case "trigger":
			cmdEngagementsTrigger(client, subArgs[1:])
		default:
			fatal("unknown engagements subcommand: %s\nUsage: aegiscli engagements <list|create|trigger>", subArgs[0])
		}

	case "runs":
		if len(subArgs) == 0 {
			fatal("missing subcommand for 'runs'\nUsage: aegiscli runs <list|get|kill>")
		}
		switch subArgs[0] {
		case "list":
			cmdRunsList(client, subArgs[1:])
		case "get":
			cmdRunsGet(client, subArgs[1:])
		case "kill":
			cmdRunsKill(client, subArgs[1:])
		default:
			fatal("unknown runs subcommand: %s\nUsage: aegiscli runs <list|get|kill>", subArgs[0])
		}

	case "findings":
		if len(subArgs) == 0 {
			fatal("missing subcommand for 'findings'\nUsage: aegiscli findings <list|get|ticket>")
		}
		switch subArgs[0] {
		case "list":
			cmdFindingsList(client, subArgs[1:])
		case "get":
			cmdFindingsGet(client, subArgs[1:])
		case "ticket":
			cmdFindingsTicket(client, subArgs[1:])
		default:
			fatal("unknown findings subcommand: %s\nUsage: aegiscli findings <list|get|ticket>", subArgs[0])
		}

	case "reports":
		if len(subArgs) == 0 {
			fatal("missing subcommand for 'reports'\nUsage: aegiscli reports <list|generate>")
		}
		switch subArgs[0] {
		case "list":
			cmdReportsList(client, subArgs[1:])
		case "generate":
			cmdReportsGenerate(client, subArgs[1:])
		default:
			fatal("unknown reports subcommand: %s\nUsage: aegiscli reports <list|generate>", subArgs[0])
		}

	case "kill-switch":
		if len(subArgs) == 0 {
			fatal("missing subcommand for 'kill-switch'\nUsage: aegiscli kill-switch <engage|disengage|status>")
		}
		switch subArgs[0] {
		case "engage":
			cmdKillSwitchEngage(client, subArgs[1:])
		case "disengage":
			cmdKillSwitchDisengage(client, subArgs[1:])
		case "status":
			cmdKillSwitchStatus(client, subArgs[1:])
		default:
			fatal("unknown kill-switch subcommand: %s\nUsage: aegiscli kill-switch <engage|disengage|status>", subArgs[0])
		}

	case "health":
		cmdHealth(client, subArgs)

	case "version":
		cmdVersion(client, subArgs)

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}
