package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

func (s *Server) upsertDeviceRegistration(ctx context.Context, userID, deviceID string, capabilities []DeviceCapability) error {
	capJSON, err := json.Marshal(capabilities)
	if err != nil {
		return fmt.Errorf("marshal capabilities: %w", err)
	}
	_, err = s.db.Exec(ctx, `
INSERT INTO device_registrations (user_id, device_id, capabilities, last_seen_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (user_id, device_id) DO UPDATE
    SET capabilities  = EXCLUDED.capabilities,
        last_seen_at  = NOW();
`, userID, deviceID, capJSON)
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
SELECT device_id, capabilities, last_seen_at
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
		if err := rows.Scan(&deviceID, &capRaw, &lastSeen); err != nil {
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
		}
	}
	return out, nil
}
