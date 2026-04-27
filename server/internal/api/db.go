package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/opencrow/opencrow/server/internal/auth"
)

func (s *Server) ensureAdminUser(ctx context.Context) (UserDTO, error) {
	if s.adminUsername == "" || s.adminPasswordBcrypt == "" {
		return UserDTO{}, errors.New("single-user admin mode requires ADMIN_USERNAME and ADMIN_PASSWORD_BCRYPT")
	}

	const q = `
INSERT INTO users (username, password_hash)
VALUES ($1, $2)
ON CONFLICT (username)
DO UPDATE SET password_hash = EXCLUDED.password_hash
RETURNING id::text, username;
`
	var user UserDTO
	err := s.db.QueryRow(ctx, q, s.adminUsername, s.adminPasswordBcrypt).Scan(&user.ID, &user.Username)
	return user, err
}

func (s *Server) findUserByID(ctx context.Context, id string) (UserDTO, string, error) {
	const q = `SELECT id::text, username, password_hash FROM users WHERE id = $1::uuid;`
	var user UserDTO
	var passwordHash string
	err := s.db.QueryRow(ctx, q, id).Scan(&user.ID, &user.Username, &passwordHash)
	return user, passwordHash, err
}

func (s *Server) createSessionAndTokens(ctx context.Context, userID, deviceLabel string) (auth.TokenPair, error) {
	// Enforce per-user session limit if configured.
	if s.maxSessionsPerUser > 0 {
		const countQ = `SELECT COUNT(*) FROM device_sessions WHERE user_id = $1::uuid;`
		var count int
		if err := s.db.QueryRow(ctx, countQ, userID).Scan(&count); err != nil {
			return auth.TokenPair{}, fmt.Errorf("count sessions: %w", err)
		}
		if count >= s.maxSessionsPerUser {
			return auth.TokenPair{}, fmt.Errorf("session limit reached (%d)", s.maxSessionsPerUser)
		}
	}

	const q = `
INSERT INTO device_sessions (user_id, device_label, refresh_token_hash)
VALUES ($1::uuid, $2, '')
RETURNING id::text;
`

	var sessionID string
	if err := s.db.QueryRow(ctx, q, userID, deviceLabel).Scan(&sessionID); err != nil {
		return auth.TokenPair{}, fmt.Errorf("insert device session: %w", err)
	}

	tokens, err := s.authMgr.NewTokenPair(userID, sessionID)
	if err != nil {
		return auth.TokenPair{}, fmt.Errorf("mint token pair: %w", err)
	}

	hash, err := hashRefreshToken(tokens.RefreshToken)
	if err != nil {
		return auth.TokenPair{}, fmt.Errorf("hash refresh token: %w", err)
	}

	if err := s.updateSessionRefreshToken(ctx, sessionID, string(hash)); err != nil {
		return auth.TokenPair{}, fmt.Errorf("update session refresh token: %w", err)
	}

	return tokens, nil
}

func (s *Server) findSession(ctx context.Context, sessionID, userID string) (sessionRow, error) {
	const q = `
SELECT id::text, user_id::text, refresh_token_hash, created_at, last_seen_at, COALESCE(device_label, '')
FROM device_sessions
WHERE id = $1::uuid AND user_id = $2::uuid;
`
	var row sessionRow
	err := s.db.QueryRow(ctx, q, sessionID, userID).Scan(
		&row.ID,
		&row.UserID,
		&row.RefreshTokenHash,
		&row.CreatedAt,
		&row.LastSeenAt,
		&row.DeviceLabel,
	)
	return row, err
}

func (s *Server) updateSessionRefreshToken(ctx context.Context, sessionID, refreshTokenHash string) error {
	const q = `
UPDATE device_sessions
SET refresh_token_hash = $2, last_seen_at = NOW()
WHERE id = $1::uuid;
`
	_, err := s.db.Exec(ctx, q, sessionID, refreshTokenHash)
	return err
}

func (s *Server) listUserSessions(ctx context.Context, userID string) ([]SessionDTO, error) {
	const q = `
SELECT id::text, COALESCE(device_label, ''), created_at, last_seen_at
FROM device_sessions
WHERE user_id = $1::uuid
ORDER BY last_seen_at DESC;
`

	rows, err := s.db.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []SessionDTO
	for rows.Next() {
		var session SessionDTO
		var createdAt time.Time
		var lastSeenAt time.Time
		if err := rows.Scan(&session.ID, &session.DeviceLabel, &createdAt, &lastSeenAt); err != nil {
			return nil, err
		}
		session.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		session.LastSeenAt = lastSeenAt.UTC().Format(time.RFC3339)
		result = append(result, session)
	}
	return result, rows.Err()
}

func (s *Server) deleteUserSession(ctx context.Context, userID, sessionID string) (bool, error) {
	const q = `DELETE FROM device_sessions WHERE id = $1::uuid AND user_id = $2::uuid;`
	cmd, err := s.db.Exec(ctx, q, sessionID, userID)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}

func (s *Server) touchSession(ctx context.Context, userID, sessionID string) error {
	const q = `
UPDATE device_sessions
SET last_seen_at = NOW()
WHERE id = $1::uuid AND user_id = $2::uuid
RETURNING id;
`
	var id string
	return s.db.QueryRow(ctx, q, sessionID, userID).Scan(&id)
}
