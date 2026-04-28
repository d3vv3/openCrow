package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

func (s *Server) upsertDeviceRegistration(ctx context.Context, userID, deviceID string, capabilities []DeviceCapability, pushEndpoint, pushAuth string) error {
	capJSON, err := json.Marshal(capabilities)
	if err != nil {
		return fmt.Errorf("marshal capabilities: %w", err)
	}
	_, err = s.db.Exec(ctx, `
INSERT INTO device_registrations (user_id, device_id, capabilities, push_endpoint, push_auth, last_seen_at)
VALUES ($1, $2, $3, $4, $5, NOW())
ON CONFLICT (user_id, device_id) DO UPDATE
    SET capabilities  = EXCLUDED.capabilities,
        push_endpoint = COALESCE(NULLIF(EXCLUDED.push_endpoint, ''), device_registrations.push_endpoint),
        push_auth     = COALESCE(NULLIF(EXCLUDED.push_auth, ''), device_registrations.push_auth),
        last_seen_at  = NOW();
`, userID, deviceID, capJSON, pushEndpoint, pushAuth)
	return err
}

// getDeviceCapabilities returns the capabilities for a specific device.
func (s *Server) getDeviceCapabilities(ctx context.Context, userID, deviceID string) ([]DeviceCapability, error) {
	var capRaw []byte
	err := s.db.QueryRow(ctx, `
SELECT capabilities FROM device_registrations
WHERE user_id = $1 AND device_id = $2;
`, userID, deviceID).Scan(&capRaw)
	if err != nil {
		return nil, err
	}
	var caps []DeviceCapability
	if err := json.Unmarshal(capRaw, &caps); err != nil {
		return nil, err
	}
	return caps, nil
}
func (s *Server) deleteDeviceRegistration(ctx context.Context, userID, deviceID string) error {
	_, err := s.db.Exec(ctx, `
DELETE FROM device_registrations WHERE user_id = $1 AND device_id = $2;
`, userID, deviceID)
	return err
}

func (s *Server) listDeviceRegistrations(ctx context.Context, userID string) (map[string]DeviceRegistrationDTO, error) {
	rows, err := s.db.Query(ctx, `
SELECT device_id, capabilities, last_seen_at, COALESCE(push_endpoint, '')
FROM device_registrations
WHERE user_id = $1;
`, userID)
	if err != nil {
		return nil, fmt.Errorf("query device registrations: %w", err)
	}
	defer rows.Close()

	out := make(map[string]DeviceRegistrationDTO)
	for rows.Next() {
		var deviceID string
		var capRaw []byte
		var lastSeen time.Time
		var pushEndpoint string
		if err := rows.Scan(&deviceID, &capRaw, &lastSeen, &pushEndpoint); err != nil {
			return nil, fmt.Errorf("scan device registration: %w", err)
		}
		var caps []DeviceCapability
		if err := json.Unmarshal(capRaw, &caps); err != nil {
			caps = nil
		}
		out[deviceID] = DeviceRegistrationDTO{
			DeviceID:     deviceID,
			Capabilities: caps,
			LastSeenAt:   lastSeen.Format(time.RFC3339),
			PushEndpoint: pushEndpoint,
		}
	}
	return out, nil
}

// getDevicePushEndpoint returns the UnifiedPush endpoint for a specific device, or "" if not set.
func (s *Server) getDevicePushEndpoint(ctx context.Context, userID, deviceID string) (string, error) {
	var ep string
	err := s.db.QueryRow(ctx, `
SELECT COALESCE(push_endpoint, '') FROM device_registrations
WHERE user_id = $1 AND device_id = $2;
`, userID, deviceID).Scan(&ep)
	return ep, err
}

// getDevicePushEndpoints returns all non-empty UnifiedPush endpoints registered for the given user.
func (s *Server) getDevicePushEndpoints(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `
SELECT push_endpoint FROM device_registrations
WHERE user_id = $1 AND push_endpoint IS NOT NULL AND push_endpoint <> '';
`, userID)
	if err != nil {
		return nil, fmt.Errorf("query push endpoints: %w", err)
	}
	defer rows.Close()
	var eps []string
	for rows.Next() {
		var ep string
		if err := rows.Scan(&ep); err != nil {
			continue
		}
		eps = append(eps, ep)
	}
	return eps, nil
}
