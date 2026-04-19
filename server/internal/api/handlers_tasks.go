package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// @Summary List all scheduled tasks
// @Tags    tasks
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string][]TaskDTO
// @Failure 401 {object} ErrorResponse
// @Router  /v1/tasks [get]
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	tasks, err := s.listTasks(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load tasks")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

// @Summary Create a new scheduled task
// @Tags    tasks
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body CreateTaskRequest true "Task definition"
// @Success 201 {object} TaskDTO
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/tasks [post]
func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var req CreateTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	description := strings.TrimSpace(req.Description)
	prompt := strings.TrimSpace(req.Prompt)
	if description == "" || prompt == "" || strings.TrimSpace(req.ExecuteAt) == "" {
		writeError(w, http.StatusBadRequest, "description, prompt, executeAt required")
		return
	}

	executeAt, err := time.Parse(time.RFC3339, req.ExecuteAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "executeAt must be RFC3339")
		return
	}

	task, err := s.createTask(r.Context(), userID, description, prompt, executeAt, req.CronExpression)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to create task")
		return
	}

	writeJSON(w, http.StatusCreated, task)
}

// @Summary Delete a task by ID
// @Tags    tasks
// @Security BearerAuth
// @Produce json
// @Param   id path string true "Task ID"
// @Success 200 {object} map[string]bool
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router  /v1/tasks/{id} [delete]
func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	taskID := r.PathValue("id")
	if !isUUID(taskID) {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}

	deleted, err := s.deleteTask(r.Context(), userID, taskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to delete task")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// @Summary Get a task by ID
// @Tags    tasks
// @Security BearerAuth
// @Produce json
// @Param   id path string true "Task ID"
// @Success 200 {object} TaskDTO
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router  /v1/tasks/{id} [get]
func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	taskID := r.PathValue("id")
	if !isUUID(taskID) {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}

	task, err := s.getTask(r.Context(), userID, taskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "unable to load task")
		return
	}

	writeJSON(w, http.StatusOK, task)
}

// @Summary Update a task by ID
// @Tags    tasks
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   id   path string            true "Task ID"
// @Param   body body UpdateTaskRequest true "Fields to update"
// @Success 200 {object} TaskDTO
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router  /v1/tasks/{id} [patch]
func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	taskID := r.PathValue("id")
	if !isUUID(taskID) {
		writeError(w, http.StatusBadRequest, "invalid task id")
		return
	}

	var req UpdateTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	task, err := s.updateTask(r.Context(), userID, taskID, req)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "unable to update task")
		return
	}

	writeJSON(w, http.StatusOK, task)
}
