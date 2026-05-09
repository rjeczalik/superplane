package hetznerrobot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/superplanehq/superplane/pkg/core"
)

const robotBaseURL = "https://robot-ws.your-server.de"

type Client struct {
	http     core.HTTPContext
	baseURL  string
	username string
	password string
}

type APIError struct {
	StatusCode int
	Body       string
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	if e.Code != "" && e.Message != "" {
		return fmt.Sprintf("Hetzner Robot API error %d %s: %s", e.StatusCode, e.Code, e.Message)
	}
	if e.Message != "" {
		return fmt.Sprintf("Hetzner Robot API error %d: %s", e.StatusCode, e.Message)
	}
	if e.Code != "" {
		return fmt.Sprintf("Hetzner Robot API error %d %s", e.StatusCode, e.Code)
	}
	return fmt.Sprintf("Hetzner Robot API error %d", e.StatusCode)
}

type Server struct {
	ServerNumber string   `json:"server_number" mapstructure:"server_number"`
	ServerName   string   `json:"server_name" mapstructure:"server_name"`
	Product      string   `json:"product" mapstructure:"product"`
	Datacenter   string   `json:"dc" mapstructure:"dc"`
	Status       string   `json:"status" mapstructure:"status"`
	Cancelled    bool     `json:"cancelled" mapstructure:"cancelled"`
	IP           []string `json:"ip" mapstructure:"ip"`
	Subnet       []struct {
		IP   string `json:"ip" mapstructure:"ip"`
		Mask string `json:"mask" mapstructure:"mask"`
	} `json:"subnet" mapstructure:"subnet"`
}

func (s *Server) DisplayName() string {
	if s.ServerName != "" {
		return s.ServerName
	}
	return fmt.Sprintf("Server %s", s.ServerNumber)
}

type ResetResult struct {
	Type   string `json:"type" mapstructure:"type"`
	Status string `json:"status" mapstructure:"status"`
}

type RescueConfig struct {
	OS            string   `json:"os" mapstructure:"os"`
	Arch          string   `json:"arch" mapstructure:"arch"`
	Active        bool     `json:"active" mapstructure:"active"`
	Password      string   `json:"password" mapstructure:"password"`
	AuthorizedKey []string `json:"authorized_key" mapstructure:"authorized_key"`
}

func NewClientFromCredentials(httpCtx core.HTTPContext, username, password string) (*Client, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}
	if password == "" {
		return nil, fmt.Errorf("password is required")
	}
	if !strings.HasPrefix(robotBaseURL, "https://") {
		return nil, fmt.Errorf("robotBaseURL must use HTTPS")
	}
	return &Client{
		http:     httpCtx,
		baseURL:  robotBaseURL,
		username: username,
		password: password,
	}, nil
}

func NewClientFromSecrets(httpCtx core.HTTPContext, secrets core.IntegrationSecretStorageReader) (*Client, error) {
	username, err := secrets.Get("username")
	if err != nil {
		return nil, fmt.Errorf("username is required: %w", err)
	}
	password, err := secrets.Get("password")
	if err != nil {
		return nil, fmt.Errorf("password is required: %w", err)
	}
	return NewClientFromCredentials(httpCtx, username, password)
}

func NewClient(httpCtx core.HTTPContext, integration core.IntegrationContext) (*Client, error) {
	if !integration.LegacySetup() {
		return NewClientFromSecrets(httpCtx, integration.Secrets())
	}
	usernameBytes, err := integration.GetConfig("username")
	if err != nil {
		return nil, fmt.Errorf("username is required: %w", err)
	}
	passwordBytes, err := integration.GetConfig("password")
	if err != nil {
		return nil, fmt.Errorf("password is required: %w", err)
	}
	return NewClientFromCredentials(httpCtx, string(usernameBytes), string(passwordBytes))
}

func (c *Client) do(method, path string, formData url.Values) (*http.Response, error) {
	var body io.Reader
	if formData != nil {
		body = strings.NewReader(formData.Encode())
	}
	req, err := http.NewRequestWithContext(context.Background(), method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")
	if formData != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return c.http.Do(req)
}

func (c *Client) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	// Body is not closed here — callers own the lifecycle via defer or manual close.
	apiErr := &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	var errPayload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &errPayload) == nil {
		apiErr.Code = errPayload.Error.Code
		apiErr.Message = errPayload.Error.Message
	}
	return apiErr
}

func decodeJSON(r io.Reader, result any) error {
	var raw any
	dec := json.NewDecoder(r)
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return err
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           result,
		TagName:          "json",
		WeaklyTypedInput: true,
	})
	if err != nil {
		return err
	}

	return decoder.Decode(raw)
}

func (c *Client) Verify() error {
	resp, err := c.do("GET", "/server?per_page=1", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return c.parseError(resp)
	}
	return nil
}

func (c *Client) ListServers() ([]Server, error) {
	all := []Server{}
	seen := map[string]struct{}{}
	page := 1

	for page <= 100 {
		resp, err := c.do("GET", fmt.Sprintf("/server?per_page=50&page=%d", page), nil)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			parseErr := c.parseError(resp)
			resp.Body.Close()
			return nil, parseErr
		}

		var out []struct {
			Server Server `json:"server" mapstructure:"server"`
		}
		if err := decodeJSON(resp.Body, &out); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode list servers response: %w", err)
		}
		resp.Body.Close()

		if len(out) == 0 {
			break
		}

		for _, entry := range out {
			if _, ok := seen[entry.Server.ServerNumber]; ok {
				continue
			}
			seen[entry.Server.ServerNumber] = struct{}{}
			all = append(all, entry.Server)
		}

		if len(out) < 50 {
			break
		}
		page++
	}

	return all, nil
}

func (c *Client) GetServer(serverNumber string) (*Server, error) {
	if err := validateServerNumber(serverNumber); err != nil {
		return nil, err
	}
	resp, err := c.do("GET", "/server/"+serverNumber, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}
	var out struct {
		Server Server `json:"server" mapstructure:"server"`
	}
	if err := decodeJSON(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("decode get server response: %w", err)
	}
	return &out.Server, nil
}

func (c *Client) ResetServer(serverNumber, resetType string) (*ResetResult, error) {
	if err := validateServerNumber(serverNumber); err != nil {
		return nil, err
	}
	formData := url.Values{}
	formData.Set("type", resetType)
	resp, err := c.do("POST", "/reset/"+serverNumber, formData)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}
	var out struct {
		Reset ResetResult `json:"reset" mapstructure:"reset"`
	}
	if err := decodeJSON(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("decode reset server response: %w", err)
	}
	return &out.Reset, nil
}

func (c *Client) EnableRescue(serverNumber, os, arch string, sshKeys []string) (*RescueConfig, error) {
	if err := validateServerNumber(serverNumber); err != nil {
		return nil, err
	}
	formData := url.Values{}
	formData.Set("os", os)
	formData.Set("arch", arch)
	for _, key := range sshKeys {
		formData.Add("authorized_key[]", key)
	}
	resp, err := c.do("POST", "/boot/"+serverNumber+"/rescue", formData)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}
	var out struct {
		Rescue RescueConfig `json:"rescue" mapstructure:"rescue"`
	}
	if err := decodeJSON(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("decode enable rescue response: %w", err)
	}
	return &out.Rescue, nil
}

func (c *Client) DisableRescue(serverNumber string) error {
	if err := validateServerNumber(serverNumber); err != nil {
		return err
	}
	resp, err := c.do("DELETE", "/boot/"+serverNumber+"/rescue", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return c.parseError(resp)
	}
	return nil
}

type SSHKey struct {
	Name        string `json:"name" mapstructure:"name"`
	Fingerprint string `json:"fingerprint" mapstructure:"fingerprint"`
	Type        string `json:"type" mapstructure:"type"`
	Size        int    `json:"size" mapstructure:"size"`
	Data        string `json:"data" mapstructure:"data"`
}

// ListSSHKeys returns all SSH keys in the account.
// The Hetzner Robot SSH key API does not paginate — it returns all keys in one response.
// Most accounts have fewer than 50 keys so this is acceptable.
func (c *Client) ListSSHKeys() ([]SSHKey, error) {
	resp, err := c.do("GET", "/key", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err := c.parseError(resp)
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound && apiErr.Code == "NOT_FOUND" {
			return []SSHKey{}, nil
		}
		return nil, err
	}
	var out []struct {
		Key SSHKey `json:"key" mapstructure:"key"`
	}
	if err := decodeJSON(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("decode list ssh keys response: %w", err)
	}
	keys := make([]SSHKey, len(out))
	for i, entry := range out {
		keys[i] = entry.Key
	}
	return keys, nil
}

func (c *Client) AddSSHKey(name, data string) (*SSHKey, error) {
	formData := url.Values{}
	formData.Set("name", name)
	formData.Set("data", data)
	resp, err := c.do("POST", "/key", formData)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, c.parseError(resp)
	}
	var out struct {
		Key SSHKey `json:"key" mapstructure:"key"`
	}
	if err := decodeJSON(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("decode add ssh key response: %w", err)
	}
	return &out.Key, nil
}

func (c *Client) DeleteSSHKey(fingerprint string) error {
	if err := validateFingerprint(fingerprint); err != nil {
		return err
	}
	resp, err := c.do("DELETE", "/key/"+url.PathEscape(fingerprint), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return c.parseError(resp)
	}
	return nil
}

// --- Linux boot configuration ---

type LinuxBootConfig struct {
	ServerNumber  string   `json:"server_number" mapstructure:"server_number"`
	Dist          []string `json:"dist" mapstructure:"dist"`
	Lang          []string `json:"lang" mapstructure:"lang"`
	Active        bool     `json:"active" mapstructure:"active"`
	Password      string   `json:"password" mapstructure:"password"`
	AuthorizedKey []string `json:"authorized_key" mapstructure:"authorized_key"`
	HostKey       []string `json:"host_key" mapstructure:"host_key"`
}

type LinuxConfig struct {
	ServerNumber  string   `json:"server_number" mapstructure:"server_number"`
	Dist          string   `json:"dist" mapstructure:"dist"`
	Lang          string   `json:"lang" mapstructure:"lang"`
	Active        bool     `json:"active" mapstructure:"active"`
	Password      string   `json:"password" mapstructure:"password"`
	AuthorizedKey []string `json:"authorized_key" mapstructure:"authorized_key"`
	HostKey       []string `json:"host_key" mapstructure:"host_key"`
}

func stringListFromAny(value any) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{v}
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return []string{fmt.Sprint(v)}
	}
}

func (c *Client) GetLinuxConfig(serverNumber string) (*LinuxBootConfig, error) {
	if err := validateServerNumber(serverNumber); err != nil {
		return nil, err
	}
	resp, err := c.do("GET", "/boot/"+serverNumber+"/linux", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}
	var out struct {
		Linux struct {
			ServerNumber  string   `json:"server_number" mapstructure:"server_number"`
			Dist          any      `json:"dist" mapstructure:"dist"`
			Lang          any      `json:"lang" mapstructure:"lang"`
			Active        bool     `json:"active" mapstructure:"active"`
			Password      string   `json:"password" mapstructure:"password"`
			AuthorizedKey []string `json:"authorized_key" mapstructure:"authorized_key"`
			HostKey       []string `json:"host_key" mapstructure:"host_key"`
		} `json:"linux" mapstructure:"linux"`
	}
	if err := decodeJSON(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("decode get linux config response: %w", err)
	}
	return &LinuxBootConfig{
		ServerNumber:  out.Linux.ServerNumber,
		Dist:          stringListFromAny(out.Linux.Dist),
		Lang:          stringListFromAny(out.Linux.Lang),
		Active:        out.Linux.Active,
		Password:      out.Linux.Password,
		AuthorizedKey: out.Linux.AuthorizedKey,
		HostKey:       out.Linux.HostKey,
	}, nil
}

func (c *Client) ActivateLinux(serverNumber, dist, lang string, authorizedKeys []string) (*LinuxConfig, error) {
	if err := validateServerNumber(serverNumber); err != nil {
		return nil, err
	}
	formData := url.Values{}
	formData.Set("dist", dist)
	formData.Set("lang", lang)
	for _, key := range authorizedKeys {
		formData.Add("authorized_key[]", key)
	}
	resp, err := c.do("POST", "/boot/"+serverNumber+"/linux", formData)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}
	var out struct {
		Linux LinuxConfig `json:"linux" mapstructure:"linux"`
	}
	if err := decodeJSON(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("decode activate linux response: %w", err)
	}
	return &out.Linux, nil
}

func (c *Client) DeactivateLinux(serverNumber string) error {
	if err := validateServerNumber(serverNumber); err != nil {
		return err
	}
	resp, err := c.do("DELETE", "/boot/"+serverNumber+"/linux", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return c.parseError(resp)
	}
	return nil
}

// --- Firewall ---

type FirewallRule struct {
	Name      string `json:"name" mapstructure:"name"`
	IPVersion string `json:"ip_version" mapstructure:"ip_version"`
	Protocol  string `json:"protocol" mapstructure:"protocol"`
	DstIP     string `json:"dst_ip" mapstructure:"dst_ip"`
	SrcIP     string `json:"src_ip" mapstructure:"src_ip"`
	DstPort   string `json:"dst_port" mapstructure:"dst_port"`
	SrcPort   string `json:"src_port" mapstructure:"src_port"`
	Action    string `json:"action" mapstructure:"action"`
	TCPFlags  string `json:"tcp_flags" mapstructure:"tcp_flags"`
}

type FirewallConfig struct {
	ServerNumber string        `json:"server_number" mapstructure:"server_number"`
	Status       string        `json:"status" mapstructure:"status"`
	WhitelistHos bool          `json:"whitelist_hos" mapstructure:"whitelist_hos"`
	Rules        FirewallRules `json:"rules" mapstructure:"rules"`
}

type FirewallRules struct {
	Input  []FirewallRule `json:"input" mapstructure:"input"`
	Output []FirewallRule `json:"output" mapstructure:"output"`
}

func addRuleField(formData url.Values, prefix, name, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	formData.Set(prefix+"["+name+"]", value)
}

func addFirewallRuleFormData(formData url.Values, direction string, index int, rule FirewallRule) {
	prefix := fmt.Sprintf("rules[%s][%d]", direction, index)
	addRuleField(formData, prefix, "name", rule.Name)
	addRuleField(formData, prefix, "ip_version", rule.IPVersion)
	addRuleField(formData, prefix, "protocol", rule.Protocol)
	addRuleField(formData, prefix, "dst_ip", rule.DstIP)
	addRuleField(formData, prefix, "src_ip", rule.SrcIP)
	addRuleField(formData, prefix, "dst_port", rule.DstPort)
	addRuleField(formData, prefix, "src_port", rule.SrcPort)
	addRuleField(formData, prefix, "action", rule.Action)
	addRuleField(formData, prefix, "tcp_flags", rule.TCPFlags)
}

func (c *Client) GetFirewall(serverNumber string) (*FirewallConfig, error) {
	if err := validateServerNumber(serverNumber); err != nil {
		return nil, err
	}
	resp, err := c.do("GET", "/firewall/"+serverNumber, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}
	var out struct {
		Firewall FirewallConfig `json:"firewall" mapstructure:"firewall"`
	}
	if err := decodeJSON(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("decode get firewall response: %w", err)
	}
	return &out.Firewall, nil
}

func (c *Client) SetFirewall(serverNumber, status string, whitelistHos bool, rules FirewallRules) (*FirewallConfig, error) {
	if err := validateServerNumber(serverNumber); err != nil {
		return nil, err
	}
	formData := url.Values{}
	formData.Set("status", status)
	formData.Set("whitelist_hos", strconv.FormatBool(whitelistHos))
	for i, rule := range rules.Input {
		addFirewallRuleFormData(formData, "input", i, rule)
	}
	for i, rule := range rules.Output {
		addFirewallRuleFormData(formData, "output", i, rule)
	}
	resp, err := c.do("POST", "/firewall/"+serverNumber, formData)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}
	var out struct {
		Firewall FirewallConfig `json:"firewall" mapstructure:"firewall"`
	}
	if err := decodeJSON(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("decode set firewall response: %w", err)
	}
	return &out.Firewall, nil
}

func (c *Client) DeleteFirewall(serverNumber string) error {
	if err := validateServerNumber(serverNumber); err != nil {
		return err
	}
	resp, err := c.do("DELETE", "/firewall/"+serverNumber, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return c.parseError(resp)
	}
	return nil
}

// --- Wake-on-LAN ---

type WOLResult struct {
	ServerIP      string `json:"server_ip" mapstructure:"server_ip"`
	ServerIPv6Net string `json:"server_ipv6_net" mapstructure:"server_ipv6_net"`
	ServerNumber  string `json:"server_number" mapstructure:"server_number"`
}

func (c *Client) SendWOL(serverNumber string) (*WOLResult, error) {
	if err := validateServerNumber(serverNumber); err != nil {
		return nil, err
	}
	resp, err := c.do("POST", "/wol/"+serverNumber, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}
	var out struct {
		WOL WOLResult `json:"wol" mapstructure:"wol"`
	}
	if err := decodeJSON(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("decode wol response: %w", err)
	}
	return &out.WOL, nil
}

// --- Rename server ---

func (c *Client) RenameServer(serverNumber, name string) (*Server, error) {
	if err := validateServerNumber(serverNumber); err != nil {
		return nil, err
	}
	formData := url.Values{}
	formData.Set("server_name", name)
	resp, err := c.do("POST", "/server/"+serverNumber, formData)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}
	var out struct {
		Server Server `json:"server" mapstructure:"server"`
	}
	if err := decodeJSON(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("decode rename server response: %w", err)
	}
	return &out.Server, nil
}

// Firewall rule CRUD: read-modify-write pattern. Safe for single-execution-per-workflow model.

func (c *Client) AddFirewallRule(serverNumber string, rule FirewallRule) (*FirewallConfig, error) {
	current, err := c.GetFirewall(serverNumber)
	if err != nil {
		return nil, fmt.Errorf("get current firewall: %w", err)
	}
	for _, existing := range current.Rules.Input {
		if existing.Name == rule.Name {
			return nil, fmt.Errorf("firewall rule %q already exists", rule.Name)
		}
	}
	newRules := FirewallRules{
		Input:  append(current.Rules.Input, rule),
		Output: current.Rules.Output,
	}
	return c.SetFirewall(serverNumber, current.Status, current.WhitelistHos, newRules)
}

func (c *Client) UpdateFirewallRule(serverNumber, ruleName string, rule FirewallRule) (*FirewallConfig, error) {
	current, err := c.GetFirewall(serverNumber)
	if err != nil {
		return nil, fmt.Errorf("get current firewall: %w", err)
	}
	found := false
	for i, existing := range current.Rules.Input {
		if existing.Name == ruleName {
			current.Rules.Input[i] = rule
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("firewall rule %q not found", ruleName)
	}
	return c.SetFirewall(serverNumber, current.Status, current.WhitelistHos, current.Rules)
}

func (c *Client) DeleteFirewallRuleByName(serverNumber, ruleName string) (*FirewallConfig, error) {
	current, err := c.GetFirewall(serverNumber)
	if err != nil {
		return nil, fmt.Errorf("get current firewall: %w", err)
	}
	filtered := make([]FirewallRule, 0, len(current.Rules.Input))
	found := false
	for _, existing := range current.Rules.Input {
		if existing.Name == ruleName {
			found = true
			continue
		}
		filtered = append(filtered, existing)
	}
	if !found {
		return nil, fmt.Errorf("firewall rule %q not found", ruleName)
	}
	newRules := FirewallRules{
		Input:  filtered,
		Output: current.Rules.Output,
	}
	return c.SetFirewall(serverNumber, current.Status, current.WhitelistHos, newRules)
}
