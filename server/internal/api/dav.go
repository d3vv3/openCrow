package api

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	ical "github.com/emersion/go-ical"
	vcard "github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
	"github.com/emersion/go-webdav/carddav"
	"github.com/google/uuid"

	"github.com/opencrow/opencrow/server/internal/configstore"
)

const defaultDAVTimeout = 15 * time.Second

type davClient struct {
	baseURL *url.URL
	observe *davObservedClient
	webdav  *webdav.Client
	caldav  *caldav.Client
	carddav *carddav.Client
}

type davHTTPObservation struct {
	Method      string
	URL         string
	Status      string
	StatusCode  int
	ContentType string
}

type davObservedClient struct {
	inner *http.Client
	mu    sync.RWMutex
	last  davHTTPObservation
}

type DAVTestResult struct {
	OK           bool              `json:"ok"`
	LatencyMs    int64             `json:"latencyMs"`
	Error        string            `json:"error,omitempty"`
	Principal    string            `json:"principal,omitempty"`
	WebDAV       *DAVDiscoveryInfo `json:"webdav,omitempty"`
	CalDAV       *DAVDiscoveryInfo `json:"caldav,omitempty"`
	CardDAV      *DAVDiscoveryInfo `json:"carddav,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
}

type DAVDiscoveryInfo struct {
	Enabled     bool              `json:"enabled"`
	HomeSet     string            `json:"homeSet,omitempty"`
	Collections []DAVCollection   `json:"collections,omitempty"`
	Entries     []DAVFileListItem `json:"entries,omitempty"`
	Error       string            `json:"error,omitempty"`
}

type DAVCollection struct {
	Path        string `json:"path"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
	CTag        string `json:"ctag,omitempty"`
}

type DAVFileListItem struct {
	Path         string `json:"path"`
	DisplayName  string `json:"displayName,omitempty"`
	ContentType  string `json:"contentType,omitempty"`
	IsCollection bool   `json:"isCollection"`
	Size         int64  `json:"size,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
}

type DAVCalendarEvent struct {
	Path        string `json:"path"`
	UID         string `json:"uid,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Description string `json:"description,omitempty"`
	Location    string `json:"location,omitempty"`
	StartsAt    string `json:"startsAt,omitempty"`
	EndsAt      string `json:"endsAt,omitempty"`
}

type DAVContact struct {
	Path     string `json:"path"`
	UID      string `json:"uid,omitempty"`
	FullName string `json:"fullName,omitempty"`
	Email    string `json:"email,omitempty"`
	Phone    string `json:"phone,omitempty"`
	Note     string `json:"note,omitempty"`
}

func (c *davObservedClient) Do(req *http.Request) (*http.Response, error) {
	resp, err := c.inner.Do(req)
	obs := davHTTPObservation{Method: req.Method, URL: req.URL.String()}
	if resp != nil {
		obs.Status = resp.Status
		obs.StatusCode = resp.StatusCode
		obs.ContentType = resp.Header.Get("Content-Type")
	}
	c.mu.Lock()
	c.last = obs
	c.mu.Unlock()
	return resp, err
}

func (c *davObservedClient) latestObservation() davHTTPObservation {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.last
}

func newDAVHTTPClient(username, password string) (webdav.HTTPClient, *davObservedClient) {
	observed := &davObservedClient{inner: &http.Client{
		Timeout: defaultDAVTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}}
	if strings.TrimSpace(username) == "" && password == "" {
		return observed, observed
	}
	return webdav.HTTPClientWithBasicAuth(observed, strings.TrimSpace(username), password), observed
}

func newDAVClient(rawURL, username, password string) (*davClient, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, fmt.Errorf("url is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("url must include scheme and host")
	}
	httpClient, observed := newDAVHTTPClient(username, password)
	wd, err := webdav.NewClient(httpClient, parsed.String())
	if err != nil {
		return nil, err
	}
	cd, err := caldav.NewClient(httpClient, parsed.String())
	if err != nil {
		return nil, err
	}
	card, err := carddav.NewClient(httpClient, parsed.String())
	if err != nil {
		return nil, err
	}
	return &davClient{baseURL: parsed, observe: observed, webdav: wd, caldav: cd, carddav: card}, nil
}

func newDAVClientFromConfig(cfg configstore.DAVConfig) (*davClient, error) {
	return newDAVClient(cfg.URL, cfg.Username, cfg.Password)
}

func (c *davClient) resolvePath(target string) string {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		if c.baseURL.Path == "" {
			return "/"
		}
		return c.baseURL.Path
	}
	if u, err := url.Parse(trimmed); err == nil && u.IsAbs() {
		return u.Path
	}
	if strings.HasPrefix(trimmed, "/") {
		return trimmed
	}
	basePath := c.baseURL.Path
	if basePath == "" {
		basePath = "/"
	}
	return path.Join(basePath, trimmed)
}

func (c *davClient) findCurrentUserPrincipal(ctx context.Context) (string, error) {
	return c.webdav.FindCurrentUserPrincipal(ctx)
}

func (c *davClient) findHomeSet(ctx context.Context, principal, kind string) (string, error) {
	if kind == "carddav" {
		return c.carddav.FindAddressBookHomeSet(ctx, principal)
	}
	return c.caldav.FindCalendarHomeSet(ctx, principal)
}

func (c *davClient) discoverCollections(ctx context.Context, homeSet, kind string) ([]DAVCollection, error) {
	if kind == "carddav" {
		books, err := c.carddav.FindAddressBooks(ctx, homeSet)
		if err != nil {
			return nil, err
		}
		collections := make([]DAVCollection, 0, len(books))
		for _, book := range books {
			collections = append(collections, DAVCollection{
				Path:        book.Path,
				DisplayName: strings.TrimSpace(book.Name),
				Description: strings.TrimSpace(book.Description),
			})
		}
		return collections, nil
	}

	calendars, err := c.caldav.FindCalendars(ctx, homeSet)
	if err != nil {
		return nil, err
	}
	collections := make([]DAVCollection, 0, len(calendars))
	for _, calendar := range calendars {
		collections = append(collections, DAVCollection{
			Path:        calendar.Path,
			DisplayName: strings.TrimSpace(calendar.Name),
			Description: strings.TrimSpace(calendar.Description),
		})
	}
	return collections, nil
}

func davFileInfoToItem(fi webdav.FileInfo) DAVFileListItem {
	displayName := strings.TrimSpace(path.Base(strings.TrimRight(fi.Path, "/")))
	if fi.Path == "/" || displayName == "." {
		displayName = "/"
	}
	item := DAVFileListItem{
		Path:         fi.Path,
		DisplayName:  displayName,
		ContentType:  fi.MIMEType,
		IsCollection: fi.IsDir,
		Size:         fi.Size,
	}
	if !fi.ModTime.IsZero() {
		item.UpdatedAt = fi.ModTime.UTC().Format(time.RFC3339)
	}
	return item
}

func (c *davClient) listFiles(ctx context.Context, target string, depth int) ([]DAVFileListItem, error) {
	resolved := c.resolvePath(target)
	if depth <= 0 {
		fi, err := c.webdav.Stat(ctx, resolved)
		if err != nil {
			return nil, err
		}
		return []DAVFileListItem{davFileInfoToItem(*fi)}, nil
	}

	entries, err := c.webdav.ReadDir(ctx, resolved, depth > 1)
	if err != nil {
		return nil, err
	}

	items := make([]DAVFileListItem, 0, len(entries))
	base := strings.TrimRight(resolved, "/")
	for _, entry := range entries {
		entryPath := strings.TrimSpace(entry.Path)
		if entryPath == "" || strings.TrimRight(entryPath, "/") == base {
			continue
		}
		items = append(items, davFileInfoToItem(entry))
	}
	return items, nil
}

func (c *davClient) remove(ctx context.Context, target string) error {
	return c.webdav.RemoveAll(ctx, c.resolvePath(target))
}

func (c *davClient) latestObservation() davHTTPObservation {
	if c == nil || c.observe == nil {
		return davHTTPObservation{}
	}
	return c.observe.latestObservation()
}

func (s *Server) loadDAVConfigsForUser(userID string) ([]configstore.DAVConfig, error) {
	if s.configStore == nil {
		return nil, fmt.Errorf("config store not configured")
	}
	cfg, err := s.configStore.GetUserConfig(userID)
	if err != nil {
		return nil, err
	}
	return cfg.Integrations.DAV, nil
}

func (s *Server) saveDAVConfigsForUser(userID string, davCfgs []configstore.DAVConfig) ([]configstore.DAVConfig, error) {
	if s.configStore == nil {
		return nil, fmt.Errorf("config store not configured")
	}
	cfg, err := s.configStore.GetUserConfig(userID)
	if err != nil {
		return nil, err
	}
	cfg.Integrations.DAV = davCfgs
	saved, err := s.configStore.PutUserConfig(userID, cfg)
	if err != nil {
		return nil, err
	}
	return saved.Integrations.DAV, nil
}

func (s *Server) resolveDAVConfigForUser(userID, davID string) (configstore.DAVConfig, error) {
	cfgs, err := s.loadDAVConfigsForUser(userID)
	if err != nil {
		return configstore.DAVConfig{}, err
	}
	if len(cfgs) == 0 {
		return configstore.DAVConfig{}, fmt.Errorf("DAV is not configured")
	}
	trimmedID := strings.TrimSpace(davID)
	if trimmedID == "" {
		if len(cfgs) == 1 {
			return cfgs[0], nil
		}
		return configstore.DAVConfig{}, fmt.Errorf("multiple DAV integrations configured; provide dav_id")
	}
	for _, cfg := range cfgs {
		if strings.EqualFold(strings.TrimSpace(cfg.ID), trimmedID) {
			return cfg, nil
		}
	}
	return configstore.DAVConfig{}, fmt.Errorf("DAV integration not found: %s", trimmedID)
}

func (s *Server) testDAVConnection(ctx context.Context, cfg configstore.DAVConfig) DAVTestResult {
	start := time.Now()
	client, err := newDAVClientFromConfig(cfg)
	if err != nil {
		return DAVTestResult{OK: false, LatencyMs: time.Since(start).Milliseconds(), Error: normalizeDAVError(err, cfg.URL, davHTTPObservation{})}
	}

	result := DAVTestResult{LatencyMs: time.Since(start).Milliseconds()}
	principal, err := client.findCurrentUserPrincipal(ctx)
	if err != nil {
		result.Error = normalizeDAVError(err, cfg.URL, client.latestObservation())
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}
	result.Principal = principal

	capabilities := make([]string, 0, 3)
	if cfg.WebDAVEnabled {
		entries, listErr := client.listFiles(ctx, "", 1)
		result.WebDAV = &DAVDiscoveryInfo{Enabled: true, Entries: entries}
		if listErr != nil {
			result.WebDAV.Error = normalizeDAVError(listErr, cfg.URL, client.latestObservation())
		} else {
			capabilities = append(capabilities, "webdav")
		}
	}

	if cfg.CalDAVEnabled {
		home, homeErr := client.findHomeSet(ctx, principal, "caldav")
		info := &DAVDiscoveryInfo{Enabled: true, HomeSet: home}
		if homeErr != nil {
			info.Error = normalizeDAVError(homeErr, cfg.URL, client.latestObservation())
		} else {
			capabilities = append(capabilities, "caldav")
			collections, colErr := client.discoverCollections(ctx, home, "caldav")
			if colErr != nil {
				info.Error = normalizeDAVError(colErr, cfg.URL, client.latestObservation())
			} else {
				info.Collections = collections
			}
		}
		result.CalDAV = info
	}

	if cfg.CardDAVEnabled {
		info := &DAVDiscoveryInfo{Enabled: true}
		if err := client.carddav.HasSupport(ctx); err != nil {
			info.Error = normalizeDAVError(err, cfg.URL, client.latestObservation())
		} else {
			home, homeErr := client.findHomeSet(ctx, principal, "carddav")
			info.HomeSet = home
			if homeErr != nil {
				info.Error = normalizeDAVError(homeErr, cfg.URL, client.latestObservation())
			} else {
				capabilities = append(capabilities, "carddav")
				collections, colErr := client.discoverCollections(ctx, home, "carddav")
				if colErr != nil {
					info.Error = normalizeDAVError(colErr, cfg.URL, client.latestObservation())
				} else {
					info.Collections = collections
				}
			}
		}
		result.CardDAV = info
	}

	result.Capabilities = capabilities
	result.OK = true
	result.LatencyMs = time.Since(start).Milliseconds()
	return result
}

func buildICSCalendar(uid, summary, description, location string, startAt, endAt time.Time) *ical.Calendar {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, "-//openCrow//DAV//EN")

	event := ical.NewEvent()
	event.Props.SetText(ical.PropUID, strings.TrimSpace(uid))
	event.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
	event.Props.SetDateTime(ical.PropDateTimeStart, startAt.UTC())
	event.Props.SetDateTime(ical.PropDateTimeEnd, endAt.UTC())
	event.Props.SetText(ical.PropSummary, strings.TrimSpace(summary))
	if strings.TrimSpace(description) != "" {
		event.Props.SetText(ical.PropDescription, strings.TrimSpace(description))
	}
	if strings.TrimSpace(location) != "" {
		event.Props.SetText(ical.PropLocation, strings.TrimSpace(location))
	}

	cal.Children = append(cal.Children, event.Component)
	return cal
}

func buildVCard(uid, fullName, email, phone, note string) vcard.Card {
	card := make(vcard.Card)
	card.SetValue(vcard.FieldVersion, "4.0")
	card.SetValue(vcard.FieldUID, strings.TrimSpace(uid))
	card.SetValue(vcard.FieldFormattedName, strings.TrimSpace(fullName))
	card.SetValue(vcard.FieldName, strings.TrimSpace(fullName)+";;;;")
	card.SetValue(vcard.FieldProductID, "-//openCrow//DAV//EN")
	card.SetRevision(time.Now().UTC())
	if strings.TrimSpace(email) != "" {
		card.SetValue(vcard.FieldEmail, strings.TrimSpace(email))
	}
	if strings.TrimSpace(phone) != "" {
		card.SetValue(vcard.FieldTelephone, strings.TrimSpace(phone))
	}
	if strings.TrimSpace(note) != "" {
		card.SetValue(vcard.FieldNote, strings.TrimSpace(note))
	}
	vcard.ToV4(card)
	return card
}

func ensureDAVConfigUsable(cfg configstore.DAVConfig) error {
	if strings.TrimSpace(cfg.URL) == "" {
		return fmt.Errorf("DAV URL is not configured")
	}
	if !cfg.Enabled {
		return fmt.Errorf("DAV integration is disabled")
	}
	return nil
}

func normalizeDAVError(err error, rawURL string, obs davHTTPObservation) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		msg = "unknown DAV error"
	}
	lower := strings.ToLower(msg)
	contentType := strings.ToLower(strings.TrimSpace(obs.ContentType))
	hint := davEndpointHint(rawURL)

	if obs.StatusCode == http.StatusOK && strings.Contains(contentType, "text/html") {
		return "DAV endpoint returned HTTP 200 with HTML instead of WebDAV/CalDAV/CardDAV data. This usually means the URL points to a web login or share page, not the actual DAV endpoint." + hint
	}
	if strings.Contains(lower, "multi-status") && strings.Contains(lower, "200 ok") {
		return "DAV endpoint did not return a WebDAV Multi-Status response; received HTTP 200 OK instead. This usually means the URL points to a normal web page or share link rather than a DAV endpoint." + hint
	}
	if obs.StatusCode == http.StatusNotFound || strings.Contains(lower, "404") {
		return "DAV endpoint returned HTTP 404 Not Found. The configured path is probably not the actual DAV collection or server base URL." + hint
	}
	if hint != "" {
		return msg + hint
	}
	return msg
}

func davEndpointHint(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	p := strings.ToLower(parsed.Path)
	switch {
	case strings.Contains(p, "/ajax/share/"):
		return " The `/ajax/share/` path looks like an Open-Xchange/App Suite share URL, not a DAV endpoint. Use the real CalDAV/CardDAV URL from the calendar or address book settings."
	case strings.Contains(p, "/caldav/"):
		return " The `/caldav/...` path can work only when it is the exact authenticated CalDAV collection URL or DAV base URL provided by the server."
	case strings.Contains(p, "/carddav/"):
		return " The `/carddav/...` path can work only when it is the exact authenticated CardDAV collection URL or DAV base URL provided by the server."
	default:
		return ""
	}
}

func normalizeDAVResourcePath(basePath, provided, ext string) string {
	trimmed := strings.TrimSpace(provided)
	if trimmed == "" {
		trimmed = uuid.NewString()
	}
	if !strings.HasSuffix(strings.ToLower(trimmed), ext) {
		trimmed += ext
	}
	if strings.HasPrefix(trimmed, "/") {
		return trimmed
	}
	return path.Join(basePath, trimmed)
}

func normalizeDAVIDFromArgs(args map[string]any) string {
	if args == nil {
		return ""
	}
	if id, ok := args["dav_id"].(string); ok {
		return strings.TrimSpace(id)
	}
	return ""
}

func parseOptionalRFC3339(raw string) (time.Time, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, false, nil
	}
	t, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return time.Time{}, false, err
	}
	return t, true, nil
}

func parseLimit(args map[string]any, key string, fallback, max int) int {
	if args == nil {
		return fallback
	}
	raw, ok := args[key]
	if !ok || raw == nil {
		return fallback
	}
	var n int
	switch v := raw.(type) {
	case float64:
		n = int(v)
	case int:
		n = v
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return fallback
		}
		n = parsed
	default:
		return fallback
	}
	if n <= 0 {
		return fallback
	}
	if n > max {
		return max
	}
	return n
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func icalPropText(props ical.Props, name string) string {
	if prop := props.Get(name); prop != nil {
		return strings.TrimSpace(prop.Value)
	}
	return ""
}

func eventFromCalendarObject(obj *caldav.CalendarObject) DAVCalendarEvent {
	if obj == nil {
		return DAVCalendarEvent{}
	}
	event := DAVCalendarEvent{Path: obj.Path}
	if obj.Data != nil {
		for _, child := range obj.Data.Children {
			if !strings.EqualFold(child.Name, "VEVENT") {
				continue
			}
			event.UID = firstNonEmpty(event.UID, icalPropText(child.Props, ical.PropUID))
			event.Summary = firstNonEmpty(event.Summary, icalPropText(child.Props, ical.PropSummary))
			event.Description = firstNonEmpty(event.Description, icalPropText(child.Props, ical.PropDescription))
			event.Location = firstNonEmpty(event.Location, icalPropText(child.Props, ical.PropLocation))
			if dt := child.Props.Get(ical.PropDateTimeStart); dt != nil {
				event.StartsAt = firstNonEmpty(event.StartsAt, dt.Value)
			}
			if dt := child.Props.Get(ical.PropDateTimeEnd); dt != nil {
				event.EndsAt = firstNonEmpty(event.EndsAt, dt.Value)
			}
			break
		}
	}
	return event
}

func contactFromAddressObject(obj *carddav.AddressObject) DAVContact {
	if obj == nil {
		return DAVContact{}
	}
	contact := DAVContact{Path: obj.Path}
	if obj.Card != nil {
		contact.UID = firstNonEmpty(contact.UID, obj.Card.PreferredValue(vcard.FieldUID))
		contact.FullName = firstNonEmpty(contact.FullName, obj.Card.PreferredValue(vcard.FieldFormattedName))
		contact.Email = firstNonEmpty(contact.Email, obj.Card.PreferredValue(vcard.FieldEmail))
		contact.Phone = firstNonEmpty(contact.Phone, obj.Card.PreferredValue(vcard.FieldTelephone))
		contact.Note = firstNonEmpty(contact.Note, obj.Card.PreferredValue(vcard.FieldNote))
	}
	return contact
}

func containsTextFold(haystack, needle string) bool {
	h := strings.ToLower(strings.TrimSpace(haystack))
	n := strings.ToLower(strings.TrimSpace(needle))
	if n == "" {
		return true
	}
	return strings.Contains(h, n)
}
