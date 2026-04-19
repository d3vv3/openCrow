package api

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/opencrow/opencrow/server/internal/configstore"
)

func (s *Server) toolQueueDeviceAction(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	targetDevice, ok := args["target_device"].(string)
	if !ok || targetDevice == "" {
		return map[string]any{"success": false, "error": "target_device is required"}, nil
	}
	instruction, ok := args["instruction"].(string)
	if !ok || instruction == "" {
		return map[string]any{"success": false, "error": "instruction is required"}, nil
	}
	dto, err := s.createDeviceTask(ctx, userID, targetDevice, instruction)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to queue task: %v", err)}, nil
	}
	return map[string]any{
		"success": true,
		"task_id": dto.ID,
		"message": fmt.Sprintf("Task successfully queued for device %s", targetDevice),
	}, nil
}

func (s *Server) toolListDevices(ctx context.Context, userID string) (map[string]any, error) {
	if s.configStore == nil {
		return map[string]any{"success": false, "error": "config store unavailable"}, nil
	}
	cfg, err := s.configStore.GetUserConfig(userID)
	if err != nil {
		return map[string]any{"success": false, "error": "failed to load config"}, nil
	}

	regs, _ := s.listDeviceRegistrations(ctx, userID) // best-effort; nil map is safe

	type deviceInfo struct {
		ID           string             `json:"id"`
		Name         string             `json:"name"`
		Label        string             `json:"label"`
		Enabled      bool               `json:"enabled"`
		Online       bool               `json:"online"`
		LastSeenAt   string             `json:"lastSeenAt,omitempty"`
		Capabilities []DeviceCapability `json:"capabilities"`
	}

	devices := make([]deviceInfo, 0, len(cfg.Integrations.CompanionApps))
	for _, app := range cfg.Integrations.CompanionApps {
		info := deviceInfo{
			ID:           app.ID,
			Name:         app.Name,
			Label:        app.Label,
			Enabled:      app.Enabled,
			Capabilities: []DeviceCapability{},
		}
		if reg, ok := regs[app.ID]; ok {
			info.LastSeenAt = reg.LastSeenAt
			info.Capabilities = reg.Capabilities
			// Consider online if seen within 10 minutes
			if t, err := time.Parse(time.RFC3339, reg.LastSeenAt); err == nil {
				info.Online = time.Since(t) < 10*time.Minute
			}
		}
		devices = append(devices, info)
	}
	return map[string]any{"success": true, "devices": devices, "count": len(devices)}, nil
}

func (s *Server) toolCreateDevice(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	if s.configStore == nil {
		return map[string]any{"success": false, "error": "config store unavailable"}, nil
	}
	name, _ := args["name"].(string)
	label, _ := args["label"].(string)
	if name == "" {
		return map[string]any{"success": false, "error": "name is required"}, nil
	}
	if label == "" {
		label = name
	}
	cfg, err := s.configStore.GetUserConfig(userID)
	if err != nil {
		return map[string]any{"success": false, "error": "failed to load config"}, nil
	}
	deviceID := "dev_" + uuid.New().String()[:8]
	cfg.Integrations.CompanionApps = append(cfg.Integrations.CompanionApps, configstore.CompanionAppConfig{
		ID:      deviceID,
		Name:    name,
		Label:   label,
		Enabled: true,
	})
	if _, err := s.configStore.PutUserConfig(userID, cfg); err != nil {
		return map[string]any{"success": false, "error": "failed to save config"}, nil
	}
	return map[string]any{
		"success":   true,
		"device_id": deviceID,
		"name":      name,
		"label":     label,
		"message":   fmt.Sprintf("Device '%s' (%s) created with ID %s. To pair it, go to the Devices tab and click 'Re-Pair' to generate a QR code.", label, name, deviceID),
	}, nil
}

func (s *Server) toolDeleteDevice(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	if s.configStore == nil {
		return map[string]any{"success": false, "error": "config store unavailable"}, nil
	}
	id, _ := args["id"].(string)
	name, _ := args["name"].(string)
	if id == "" && name == "" {
		return map[string]any{"success": false, "error": "id or name is required"}, nil
	}
	cfg, err := s.configStore.GetUserConfig(userID)
	if err != nil {
		return map[string]any{"success": false, "error": "failed to load config"}, nil
	}
	var kept []configstore.CompanionAppConfig
	found := false
	for _, app := range cfg.Integrations.CompanionApps {
		if (id != "" && app.ID == id) || (name != "" && app.Name == name) {
			found = true
			continue
		}
		kept = append(kept, app)
	}
	if !found {
		return map[string]any{"success": false, "error": "device not found"}, nil
	}
	cfg.Integrations.CompanionApps = kept
	if _, err := s.configStore.PutUserConfig(userID, cfg); err != nil {
		return map[string]any{"success": false, "error": "failed to save config"}, nil
	}
	return map[string]any{"success": true, "message": "Device deleted"}, nil
}

func (s *Server) toolEditDevice(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	if s.configStore == nil {
		return map[string]any{"success": false, "error": "config store unavailable"}, nil
	}
	id, _ := args["id"].(string)
	if id == "" {
		return map[string]any{"success": false, "error": "id is required"}, nil
	}
	cfg, err := s.configStore.GetUserConfig(userID)
	if err != nil {
		return map[string]any{"success": false, "error": "failed to load config"}, nil
	}
	found := false
	for i, app := range cfg.Integrations.CompanionApps {
		if app.ID != id {
			continue
		}
		if newName, ok := args["name"].(string); ok && newName != "" {
			cfg.Integrations.CompanionApps[i].Name = newName
		}
		if newLabel, ok := args["label"].(string); ok && newLabel != "" {
			cfg.Integrations.CompanionApps[i].Label = newLabel
		}
		if enabled, ok := args["enabled"].(bool); ok {
			cfg.Integrations.CompanionApps[i].Enabled = enabled
		}
		found = true
		break
	}
	if !found {
		return map[string]any{"success": false, "error": "device not found"}, nil
	}
	if _, err := s.configStore.PutUserConfig(userID, cfg); err != nil {
		return map[string]any{"success": false, "error": "failed to save config"}, nil
	}
	return map[string]any{"success": true, "message": "Device updated"}, nil
}

func (s *Server) toolGetDeviceCapabilities(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	id, _ := args["id"].(string)
	if id == "" {
		return map[string]any{"success": false, "error": "id is required"}, nil
	}
	regs, err := s.listDeviceRegistrations(ctx, userID)
	if err != nil {
		return map[string]any{"success": false, "error": "failed to load registrations"}, nil
	}
	reg, ok := regs[id]
	if !ok {
		return map[string]any{"success": true, "id": id, "capabilities": []any{}, "online": false, "message": "Device has not registered any capabilities yet"}, nil
	}
	online := false
	if t, err := time.Parse(time.RFC3339, reg.LastSeenAt); err == nil {
		online = time.Since(t) < 10*time.Minute
	}
	return map[string]any{
		"success":      true,
		"id":           id,
		"capabilities": reg.Capabilities,
		"last_seen_at": reg.LastSeenAt,
		"online":       online,
	}, nil
}

func (s *Server) toolListDeviceTasks(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	target, _ := args["target_device"].(string)
	tasks, err := s.listDeviceTasks(ctx, userID)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to list tasks: %v", err)}, nil
	}
	if target != "" {
		var filtered []DeviceTaskDTO
		for _, t := range tasks {
			if t.TargetDevice == target {
				filtered = append(filtered, t)
			}
		}
		tasks = filtered
	}
	if tasks == nil {
		tasks = []DeviceTaskDTO{}
	}
	return map[string]any{"success": true, "tasks": tasks, "count": len(tasks)}, nil
}
