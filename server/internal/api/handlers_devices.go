package api

import (
	"net/http"
)

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

func (s *Server) handleDeleteDeviceTask(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	taskID := r.PathValue("id")
	if err := s.deleteDeviceTask(r.Context(), userID, taskID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete task")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
