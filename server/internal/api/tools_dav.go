package api

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/emersion/go-webdav/caldav"
	"github.com/emersion/go-webdav/carddav"
	"github.com/google/uuid"

	"github.com/opencrow/opencrow/server/internal/configstore"
)

func (s *Server) toolSetupDAV(_ context.Context, userID string, args map[string]any) (map[string]any, error) {
	url, _ := args["url"].(string)
	url = strings.TrimSpace(url)
	if url == "" {
		return map[string]any{"success": false, "error": "url is required"}, nil
	}
	configs, err := s.loadDAVConfigsForUser(userID)
	if err != nil {
		return map[string]any{"success": false, "error": "failed to load config"}, nil
	}
	targetID := normalizeDAVIDFromArgs(args)
	targetIndex := -1
	if targetID != "" {
		for i := range configs {
			if strings.EqualFold(strings.TrimSpace(configs[i].ID), targetID) {
				targetIndex = i
				break
			}
		}
		if targetIndex < 0 {
			return map[string]any{"success": false, "error": "dav_id not found"}, nil
		}
	}

	current := configstore.DAVConfig{Enabled: true, WebDAVEnabled: true, CalDAVEnabled: true, CardDAVEnabled: true, PollIntervalSeconds: 900}
	if targetIndex >= 0 {
		current = configs[targetIndex]
	}
	if current.ID == "" {
		current.ID = uuid.NewString()
	}
	if name, ok := args["name"].(string); ok && strings.TrimSpace(name) != "" {
		current.Name = strings.TrimSpace(name)
	} else if strings.TrimSpace(current.Name) == "" {
		current.Name = fmt.Sprintf("DAV %d", len(configs)+1)
	}
	current.URL = url
	if username, ok := args["username"].(string); ok {
		current.Username = strings.TrimSpace(username)
	}
	if password, ok := args["password"].(string); ok {
		current.Password = password
	}
	if enabled, ok := args["enabled"].(bool); ok {
		current.Enabled = enabled
	} else if !current.Enabled {
		current.Enabled = true
	}
	if enabled, ok := args["webdav_enabled"].(bool); ok {
		current.WebDAVEnabled = enabled
	}
	if enabled, ok := args["caldav_enabled"].(bool); ok {
		current.CalDAVEnabled = enabled
	}
	if enabled, ok := args["carddav_enabled"].(bool); ok {
		current.CardDAVEnabled = enabled
	}
	if poll, ok := args["poll_interval_seconds"].(float64); ok && int(poll) > 0 {
		current.PollIntervalSeconds = int(poll)
	}
	if targetIndex >= 0 {
		configs[targetIndex] = current
	} else {
		configs = append(configs, current)
	}
	saved, err := s.saveDAVConfigsForUser(userID, configs)
	if err != nil {
		return map[string]any{"success": false, "error": "failed to save config"}, nil
	}
	var savedCurrent configstore.DAVConfig
	for _, cfg := range saved {
		if cfg.ID == current.ID {
			savedCurrent = cfg
			break
		}
	}
	return map[string]any{
		"success": true,
		"message": "DAV integration configured.",
		"dav":     savedCurrent,
		"davs":    saved,
	}, nil
}

func (s *Server) toolListDAVIntegrations(_ context.Context, userID string, _ map[string]any) (map[string]any, error) {
	configs, err := s.loadDAVConfigsForUser(userID)
	if err != nil {
		return map[string]any{"success": false, "error": "failed to load DAV configs"}, nil
	}
	items := make([]map[string]any, 0, len(configs))
	for _, cfg := range configs {
		items = append(items, map[string]any{
			"id":                  cfg.ID,
			"name":                cfg.Name,
			"url":                 cfg.URL,
			"enabled":             cfg.Enabled,
			"webdavEnabled":       cfg.WebDAVEnabled,
			"caldavEnabled":       cfg.CalDAVEnabled,
			"carddavEnabled":      cfg.CardDAVEnabled,
			"pollIntervalSeconds": cfg.PollIntervalSeconds,
		})
	}
	return map[string]any{"success": true, "davIntegrations": items, "count": len(items)}, nil
}

func (s *Server) toolTestDAVConnection(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	davCfg, err := s.resolveDAVConfigForUser(userID, normalizeDAVIDFromArgs(args))
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	if err := ensureDAVConfigUsable(davCfg); err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	res := s.testDAVConnection(ctx, davCfg)
	if !res.OK {
		return map[string]any{"success": false, "error": res.Error, "result": res}, nil
	}
	return map[string]any{"success": true, "result": res}, nil
}

// toolInspectDAV combines list + optional test into one call, reducing round-trips.
// When dav_id is omitted it lists all integrations. When dav_id is provided it also
// tests the connection and discovers available calendars/address books.
func (s *Server) toolInspectDAV(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	davID := normalizeDAVIDFromArgs(args)

	// Always list all integrations
	configs, err := s.loadDAVConfigsForUser(userID)
	if err != nil {
		return map[string]any{"success": false, "error": "failed to load DAV configs. Use setup_dav to add an integration first."}, nil
	}
	items := make([]map[string]any, 0, len(configs))
	for _, cfg := range configs {
		items = append(items, map[string]any{
			"dav_id":         cfg.ID,
			"name":           cfg.Name,
			"url":            cfg.URL,
			"enabled":        cfg.Enabled,
			"webdav_enabled": cfg.WebDAVEnabled,
			"caldav_enabled": cfg.CalDAVEnabled,
			"carddav_enabled": cfg.CardDAVEnabled,
		})
	}
	result := map[string]any{
		"success":     true,
		"integrations": items,
		"count":       len(items),
	}

	// If a specific dav_id was requested, also test that connection
	if davID != "" {
		davCfg, err := s.resolveDAVConfigForUser(userID, davID)
		if err != nil {
			result["test_error"] = fmt.Sprintf("dav_id %q not found. Available IDs: see integrations list above.", davID)
			return result, nil
		}
		if err := ensureDAVConfigUsable(davCfg); err != nil {
			result["test_error"] = err.Error()
			return result, nil
		}
		res := s.testDAVConnection(ctx, davCfg)
		result["test_result"] = res
		if !res.OK {
			result["test_error"] = res.Error
		}
	}
	return result, nil
}

func (s *Server) toolListWebDAVFiles(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	client, cfg, errResult := s.requireDAVClient(userID, normalizeDAVIDFromArgs(args))
	if errResult != nil {
		return errResult, nil
	}
	if !cfg.WebDAVEnabled {
		return map[string]any{"success": false, "error": "WebDAV support is disabled"}, nil
	}
	target, _ := args["path"].(string)
	depth := 1
	if raw, ok := args["depth"].(float64); ok && int(raw) >= 0 {
		depth = int(raw)
	}
	items, err := client.listFiles(ctx, target, depth)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	return map[string]any{"success": true, "path": client.resolvePath(target), "entries": items}, nil
}

func (s *Server) toolListCalDAVCalendars(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	client, cfg, errResult := s.requireDAVClient(userID, normalizeDAVIDFromArgs(args))
	if errResult != nil {
		return errResult, nil
	}
	if !cfg.CalDAVEnabled {
		return map[string]any{"success": false, "error": "CalDAV support is disabled"}, nil
	}
	principal, err := client.findCurrentUserPrincipal(ctx)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	home, err := client.findHomeSet(ctx, principal, "caldav")
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	collections, err := client.discoverCollections(ctx, home, "caldav")
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	return map[string]any{"success": true, "principal": principal, "homeSet": home, "calendars": collections}, nil
}

func (s *Server) toolListCardDAVAddressBooks(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	client, cfg, errResult := s.requireDAVClient(userID, normalizeDAVIDFromArgs(args))
	if errResult != nil {
		return errResult, nil
	}
	if !cfg.CardDAVEnabled {
		return map[string]any{"success": false, "error": "CardDAV support is disabled"}, nil
	}
	principal, err := client.findCurrentUserPrincipal(ctx)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	if err := client.carddav.HasSupport(ctx); err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	home, err := client.findHomeSet(ctx, principal, "carddav")
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	collections, err := client.discoverCollections(ctx, home, "carddav")
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	return map[string]any{"success": true, "principal": principal, "homeSet": home, "addressBooks": collections}, nil
}

func (s *Server) toolCreateCalDAVEvent(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	client, cfg, errResult := s.requireDAVClient(userID, normalizeDAVIDFromArgs(args))
	if errResult != nil {
		return errResult, nil
	}
	if !cfg.CalDAVEnabled {
		return map[string]any{"success": false, "error": "CalDAV support is disabled"}, nil
	}
	calendarPath, _ := args["calendar_path"].(string)
	calendarPath = strings.TrimSpace(calendarPath)
	if calendarPath == "" {
		return map[string]any{"success": false, "error": "calendar_path is required"}, nil
	}
	summary, _ := args["summary"].(string)
	if strings.TrimSpace(summary) == "" {
		return map[string]any{"success": false, "error": "summary is required"}, nil
	}
	startsAtRaw, _ := args["starts_at"].(string)
	endsAtRaw, _ := args["ends_at"].(string)
	startsAt, hasStart, err := parseOptionalRFC3339(startsAtRaw)
	if err != nil || !hasStart {
		return map[string]any{"success": false, "error": "starts_at is required. Use RFC3339 (e.g. \"2025-05-02T09:00:00Z\"), datetime without timezone (\"2025-05-02T09:00:00\"), or date-only (\"2025-05-02\")."}, nil
	}
	endsAt, hasEnd, err := parseOptionalRFC3339(endsAtRaw)
	if err != nil || !hasEnd {
		return map[string]any{"success": false, "error": "ends_at is required. Use RFC3339 (e.g. \"2025-05-02T10:00:00Z\"), datetime without timezone (\"2025-05-02T10:00:00\"), or date-only (\"2025-05-02\")."}, nil
	}
	if !endsAt.After(startsAt) {
		return map[string]any{"success": false, "error": "ends_at must be after starts_at"}, nil
	}
	uid, _ := args["uid"].(string)
	uid = strings.TrimSpace(uid)
	if uid == "" {
		uid = uuid.NewString()
	}
	description, _ := args["description"].(string)
	location, _ := args["location"].(string)
	calendar := buildICSCalendar(uid, summary, description, location, startsAt, endsAt)
	resourcePath := normalizeDAVResourcePath(calendarPath, uid, ".ics")
	stored, err := client.caldav.PutCalendarObject(ctx, resourcePath, calendar)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	return map[string]any{"success": true, "uid": uid, "eventPath": stored.Path, "calendarPath": path.Clean(calendarPath)}, nil
}

func (s *Server) toolDeleteCalDAVEvent(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	client, cfg, errResult := s.requireDAVClient(userID, normalizeDAVIDFromArgs(args))
	if errResult != nil {
		return errResult, nil
	}
	if !cfg.CalDAVEnabled {
		return map[string]any{"success": false, "error": "CalDAV support is disabled"}, nil
	}
	eventPath, _ := args["event_path"].(string)
	eventPath = strings.TrimSpace(eventPath)
	if eventPath == "" {
		return map[string]any{"success": false, "error": "event_path is required"}, nil
	}
	if err := client.remove(ctx, eventPath); err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	return map[string]any{"success": true, "deleted": true, "eventPath": client.resolvePath(eventPath)}, nil
}

func (s *Server) toolCreateCardDAVContact(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	client, cfg, errResult := s.requireDAVClient(userID, normalizeDAVIDFromArgs(args))
	if errResult != nil {
		return errResult, nil
	}
	if !cfg.CardDAVEnabled {
		return map[string]any{"success": false, "error": "CardDAV support is disabled"}, nil
	}
	addressBookPath, _ := args["address_book_path"].(string)
	addressBookPath = strings.TrimSpace(addressBookPath)
	if addressBookPath == "" {
		return map[string]any{"success": false, "error": "address_book_path is required"}, nil
	}
	fullName, _ := args["full_name"].(string)
	if strings.TrimSpace(fullName) == "" {
		return map[string]any{"success": false, "error": "full_name is required"}, nil
	}
	uid, _ := args["uid"].(string)
	uid = strings.TrimSpace(uid)
	if uid == "" {
		uid = uuid.NewString()
	}
	email, _ := args["email"].(string)
	phone, _ := args["phone"].(string)
	note, _ := args["note"].(string)
	card := buildVCard(uid, fullName, email, phone, note)
	resourcePath := normalizeDAVResourcePath(addressBookPath, uid, ".vcf")
	stored, err := client.carddav.PutAddressObject(ctx, resourcePath, card)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	return map[string]any{"success": true, "uid": uid, "contactPath": stored.Path, "addressBookPath": path.Clean(addressBookPath)}, nil
}

func (s *Server) toolDeleteCardDAVContact(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	client, cfg, errResult := s.requireDAVClient(userID, normalizeDAVIDFromArgs(args))
	if errResult != nil {
		return errResult, nil
	}
	if !cfg.CardDAVEnabled {
		return map[string]any{"success": false, "error": "CardDAV support is disabled"}, nil
	}
	contactPath, _ := args["contact_path"].(string)
	contactPath = strings.TrimSpace(contactPath)
	if contactPath == "" {
		return map[string]any{"success": false, "error": "contact_path is required"}, nil
	}
	if err := client.remove(ctx, contactPath); err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	return map[string]any{"success": true, "deleted": true, "contactPath": client.resolvePath(contactPath)}, nil
}

func (s *Server) requireDAVClient(userID, davID string) (*davClient, configstore.DAVConfig, map[string]any) {
	davCfg, err := s.resolveDAVConfigForUser(userID, davID)
	if err != nil {
		return nil, configstore.DAVConfig{}, map[string]any{"success": false, "error": err.Error()}
	}
	if err := ensureDAVConfigUsable(davCfg); err != nil {
		return nil, davCfg, map[string]any{"success": false, "error": err.Error()}
	}
	client, err := newDAVClientFromConfig(davCfg)
	if err != nil {
		return nil, davCfg, map[string]any{"success": false, "error": fmt.Sprintf("invalid DAV config: %v", err)}
	}
	return client, davCfg, nil
}

func (s *Server) toolListCalDAVEvents(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	client, cfg, errResult := s.requireDAVClient(userID, normalizeDAVIDFromArgs(args))
	if errResult != nil {
		return errResult, nil
	}
	if !cfg.CalDAVEnabled {
		return map[string]any{"success": false, "error": "CalDAV support is disabled"}, nil
	}
	calendarPath, _ := args["calendar_path"].(string)
	calendarPath = strings.TrimSpace(calendarPath)
	if calendarPath == "" {
		return map[string]any{"success": false, "error": "calendar_path is required"}, nil
	}
	startsAt, hasStart, err := parseOptionalRFC3339(fmt.Sprint(args["starts_at"]))
	if err != nil {
		return map[string]any{"success": false, "error": "starts_at must be RFC3339"}, nil
	}
	endsAt, hasEnd, err := parseOptionalRFC3339(fmt.Sprint(args["ends_at"]))
	if err != nil {
		return map[string]any{"success": false, "error": "ends_at must be RFC3339"}, nil
	}
	if hasStart && hasEnd && !endsAt.After(startsAt) {
		return map[string]any{"success": false, "error": "ends_at must be after starts_at"}, nil
	}
	query := &caldav.CalendarQuery{CompRequest: caldav.CalendarCompRequest{AllComps: true, AllProps: true}}
	if hasStart || hasEnd {
		query.CompFilter = caldav.CompFilter{Name: "VCALENDAR", Comps: []caldav.CompFilter{{Name: "VEVENT", Start: startsAt, End: endsAt}}}
	}
	objects, err := client.caldav.QueryCalendar(ctx, calendarPath, query)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	limit := parseLimit(args, "limit", 50, 200)
	events := make([]DAVCalendarEvent, 0, len(objects))
	for _, obj := range objects {
		events = append(events, eventFromCalendarObject(&obj))
		if len(events) >= limit {
			break
		}
	}
	return map[string]any{"success": true, "calendarPath": path.Clean(calendarPath), "events": events}, nil
}

func (s *Server) toolGetCalDAVEvent(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	client, cfg, errResult := s.requireDAVClient(userID, normalizeDAVIDFromArgs(args))
	if errResult != nil {
		return errResult, nil
	}
	if !cfg.CalDAVEnabled {
		return map[string]any{"success": false, "error": "CalDAV support is disabled"}, nil
	}
	eventPath, _ := args["event_path"].(string)
	eventPath = strings.TrimSpace(eventPath)
	if eventPath == "" {
		return map[string]any{"success": false, "error": "event_path is required"}, nil
	}
	obj, err := client.caldav.GetCalendarObject(ctx, eventPath)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	return map[string]any{"success": true, "event": eventFromCalendarObject(obj)}, nil
}

func (s *Server) toolSearchCalDAVEvents(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	queryText, _ := args["query"].(string)
	queryText = strings.TrimSpace(queryText)
	if queryText == "" {
		return map[string]any{"success": false, "error": "query is required"}, nil
	}
	listResult, err := s.toolListCalDAVEvents(ctx, userID, args)
	if err != nil {
		return nil, err
	}
	if success, _ := listResult["success"].(bool); !success {
		return listResult, nil
	}
	raw, _ := listResult["events"].([]DAVCalendarEvent)
	filtered := make([]DAVCalendarEvent, 0, len(raw))
	for _, ev := range raw {
		if containsTextFold(ev.Summary, queryText) || containsTextFold(ev.Description, queryText) || containsTextFold(ev.Location, queryText) || containsTextFold(ev.UID, queryText) {
			filtered = append(filtered, ev)
		}
	}
	listResult["events"] = filtered
	listResult["count"] = len(filtered)
	return listResult, nil
}

func (s *Server) toolListCardDAVContacts(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	client, cfg, errResult := s.requireDAVClient(userID, normalizeDAVIDFromArgs(args))
	if errResult != nil {
		return errResult, nil
	}
	if !cfg.CardDAVEnabled {
		return map[string]any{"success": false, "error": "CardDAV support is disabled"}, nil
	}
	addressBookPath, _ := args["address_book_path"].(string)
	addressBookPath = strings.TrimSpace(addressBookPath)
	if addressBookPath == "" {
		return map[string]any{"success": false, "error": "address_book_path is required"}, nil
	}
	objects, err := client.carddav.QueryAddressBook(ctx, addressBookPath, &carddav.AddressBookQuery{DataRequest: carddav.AddressDataRequest{AllProp: true}})
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	limit := parseLimit(args, "limit", 100, 500)
	contacts := make([]DAVContact, 0, len(objects))
	for _, obj := range objects {
		contacts = append(contacts, contactFromAddressObject(&obj))
		if len(contacts) >= limit {
			break
		}
	}
	return map[string]any{"success": true, "addressBookPath": path.Clean(addressBookPath), "contacts": contacts}, nil
}

func (s *Server) toolGetCardDAVContact(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	client, cfg, errResult := s.requireDAVClient(userID, normalizeDAVIDFromArgs(args))
	if errResult != nil {
		return errResult, nil
	}
	if !cfg.CardDAVEnabled {
		return map[string]any{"success": false, "error": "CardDAV support is disabled"}, nil
	}
	contactPath, _ := args["contact_path"].(string)
	contactPath = strings.TrimSpace(contactPath)
	if contactPath == "" {
		return map[string]any{"success": false, "error": "contact_path is required"}, nil
	}
	obj, err := client.carddav.GetAddressObject(ctx, contactPath)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	return map[string]any{"success": true, "contact": contactFromAddressObject(obj)}, nil
}

func (s *Server) toolSearchCardDAVContacts(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	queryText, _ := args["query"].(string)
	queryText = strings.TrimSpace(queryText)
	if queryText == "" {
		return map[string]any{"success": false, "error": "query is required"}, nil
	}
	listResult, err := s.toolListCardDAVContacts(ctx, userID, args)
	if err != nil {
		return nil, err
	}
	if success, _ := listResult["success"].(bool); !success {
		return listResult, nil
	}
	raw, _ := listResult["contacts"].([]DAVContact)
	filtered := make([]DAVContact, 0, len(raw))
	for _, contact := range raw {
		if containsTextFold(contact.FullName, queryText) || containsTextFold(contact.Email, queryText) || containsTextFold(contact.Phone, queryText) || containsTextFold(contact.Note, queryText) || containsTextFold(contact.UID, queryText) {
			filtered = append(filtered, contact)
		}
	}
	listResult["contacts"] = filtered
	listResult["count"] = len(filtered)
	return listResult, nil
}
