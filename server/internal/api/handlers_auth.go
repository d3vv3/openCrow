package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password required")
		return
	}

	if req.Username != s.adminUsername {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(s.adminPasswordBcrypt), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	user, err := s.ensureAdminUser(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to bootstrap admin user")
		return
	}

	deviceLabel := chooseDeviceLabel(req.Device, r.Header.Get("X-Device-Label"))
	tokens, err := s.createSessionAndTokens(ctx, user.ID, deviceLabel)
	if err != nil {
		log.Printf("login createSessionAndTokens failed user_id=%s device_label=%q err=%v", user.ID, deviceLabel, err)
		writeError(w, http.StatusInternalServerError, "unable to create session")
		return
	}

	writeJSON(w, http.StatusOK, AuthResponse{User: user, Tokens: tokens})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req RefreshRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	claims, err := s.authMgr.Parse(req.RefreshToken, "refresh")
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	if claims.SessionID == "" {
		writeError(w, http.StatusUnauthorized, "invalid refresh token session")
		return
	}

	session, err := s.findSession(ctx, claims.SessionID, claims.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "unable to refresh")
		return
	}

	if err := verifyRefreshTokenHash(session.RefreshTokenHash, req.RefreshToken); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	user, _, err := s.findUserByID(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "unable to refresh")
		return
	}

	tokens, err := s.authMgr.NewTokenPair(user.ID, session.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to sign tokens")
		return
	}

	hash, err := hashRefreshToken(tokens.RefreshToken)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to hash refresh token")
		return
	}

	if err := s.updateSessionRefreshToken(ctx, session.ID, string(hash)); err != nil {
		writeError(w, http.StatusInternalServerError, "unable to update session")
		return
	}

	writeJSON(w, http.StatusOK, AuthResponse{User: user, Tokens: tokens})
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	sessions, err := s.listUserSessions(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load sessions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	sessionID := r.PathValue("id")
	if !isUUID(sessionID) {
		writeError(w, http.StatusBadRequest, "invalid session id")
		return
	}

	deleted, err := s.deleteUserSession(r.Context(), userID, sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to delete session")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	sessions, err := s.listUserSessions(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load devices")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": sessions})
}

func (s *Server) requireAccessToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		// Fallback: allow token via query param for WebSocket upgrades (browsers can't set headers)
		if token == "" {
			token = r.URL.Query().Get("token")
		}
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}

		claims, err := s.authMgr.Parse(token, "access")
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid access token")
			return
		}
		if claims.SessionID == "" {
			writeError(w, http.StatusUnauthorized, "invalid access token session")
			return
		}

		if err := s.touchSession(r.Context(), claims.UserID, claims.SessionID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusUnauthorized, "session not found")
				return
			}
			writeError(w, http.StatusUnauthorized, "invalid session")
			return
		}

		ctx := context.WithValue(r.Context(), userIDContextKey, claims.UserID)
		ctx = context.WithValue(ctx, sessionIDContextKey, claims.SessionID)
		if tz := strings.TrimSpace(r.Header.Get("X-Client-Timezone")); tz != "" {
			ctx = context.WithValue(ctx, clientTimezoneContextKey, tz)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func userIDFromContext(ctx context.Context) string {
	userID, _ := ctx.Value(userIDContextKey).(string)
	return userID
}

func clientTimezoneFromContext(ctx context.Context) string {
	tz, _ := ctx.Value(clientTimezoneContextKey).(string)
	return strings.TrimSpace(tz)
}
