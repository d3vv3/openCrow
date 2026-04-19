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

// listDeviceRegistrations returns a map of deviceID → registration for a user.
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
