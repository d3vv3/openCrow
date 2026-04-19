package api

import (
	"net/http"
)

// @Summary List device registrations for the current user
// @Tags    devices
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string][]DeviceRegistrationDTO
// @Failure 401 {object} ErrorResponse
// @Router  /v1/devices/registrations [get]
func (s *Server) handleListDeviceRegistrations(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	regs, err := s.listDeviceRegistrations(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load registrations")
		return
	}
	list := make([]DeviceRegistrationDTO, 0, len(regs))
	for _, r := range regs {
		list = append(list, r)
	}
	writeJSON(w, http.StatusOK, map[string]any{"registrations": list})
}

// @Summary Register a device with its capabilities
// @Tags    devices
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   id   path string                true "Device ID (session ID)"
// @Param   body body RegisterDeviceRequest true "Device capabilities"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/devices/{id}/register [post]
func (s *Server) handleRegisterDevice(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	deviceID := r.PathValue("id")
	if deviceID == "" {
		writeError(w, http.StatusBadRequest, "device id is required")
		return
	}

	var req RegisterDeviceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Capabilities == nil {
		req.Capabilities = []DeviceCapability{}
	}

	if err := s.upsertDeviceRegistration(r.Context(), userID, deviceID, req.Capabilities); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to register device")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"deviceId":     deviceID,
		"capabilities": req.Capabilities,
		"status":       "registered",
	})
}

// @Summary List device tasks (optionally filtered by target device)
// @Tags    devices
// @Security BearerAuth
// @Produce json
// @Param   target query string false "Filter by target device ID"
// @Success 200 {object} map[string][]DeviceTaskDTO
// @Failure 401 {object} ErrorResponse
// @Router  /v1/devices/tasks [get]
func (s *Server) handleListDeviceTasks(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	target := r.URL.Query().Get("target")

	var tasks []DeviceTaskDTO
	var err error
	if target != "" {
		tasks, err = s.pollDeviceTasks(r.Context(), userID, target)
	} else {
		tasks, err = s.listDeviceTasks(r.Context(), userID)
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get device tasks")
		return
	}
	if tasks == nil {
		tasks = []DeviceTaskDTO{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

// @Summary Create a task for a target device
// @Tags    devices
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body CreateDeviceTaskRequest true "Target device and instruction"
// @Success 201 {object} DeviceTaskDTO
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/devices/tasks [post]
func (s *Server) handleCreateDeviceTask(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var req CreateDeviceTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.TargetDevice == "" || req.Instruction == "" {
		writeError(w, http.StatusBadRequest, "targetDevice and instruction are required")
		return
	}
	dto, err := s.createDeviceTask(r.Context(), userID, req.TargetDevice, req.Instruction)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}
	writeJSON(w, http.StatusCreated, dto)
}

// @Summary Mark a device task as complete
// @Tags    devices
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   id   path string                    true "Task ID"
// @Param   body body CompleteDeviceTaskRequest true "Success flag and output"
// @Success 200 {object} map[string]string
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/devices/tasks/{id}/complete [post]
func (s *Server) handleCompleteDeviceTask(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	taskID := r.PathValue("id")
	var req CompleteDeviceTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.completeDeviceTask(r.Context(), userID, taskID, req.Success, req.Output); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to complete task")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// @Summary Delete a device task by ID
// @Tags    devices
// @Security BearerAuth
// @Produce json
// @Param   id path string true "Task ID"
// @Success 204
// @Failure 401 {object} ErrorResponse
// @Router  /v1/devices/tasks/{id} [delete]
func (s *Server) handleDeleteDeviceTask(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	taskID := r.PathValue("id")
	if err := s.deleteDeviceTask(r.Context(), userID, taskID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete task")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
