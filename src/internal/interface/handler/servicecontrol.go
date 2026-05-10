package handler

import (
	"net/http"
	"os/exec"
	"strings"
	"time"

	"api-gateway/config"
	"api-gateway/internal/gateway"

	"github.com/gin-gonic/gin"
)

// ServiceControlHandler handles service management (start/stop/restart) and DB status checks.
type ServiceControlHandler struct {
	conf *config.Config
}

// NewServiceControlHandler creates a new service control handler.
func NewServiceControlHandler(conf *config.Config) *ServiceControlHandler {
	return &ServiceControlHandler{conf: conf}
}

// allowedActions lists valid systemctl actions.
var allowedActions = map[string]bool{
	"start":   true,
	"stop":    true,
	"restart": true,
	"status":  true,
}

// controlRequest is the JSON body for service control.
type controlRequest struct {
	Action string `json:"action" binding:"required"`
}

// ControlService executes systemctl action for a named service.
// POST /gateway/api/service/:name/control
// Body: {"action": "start"|"stop"|"restart"|"status"}
func (h *ServiceControlHandler) ControlService(c *gin.Context) {
	svcName := c.Param("name")
	if strings.TrimSpace(svcName) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "service name required"})
		return
	}

	// Validate service exists in config
	found := false
	for _, svc := range h.conf.Services {
		if svc.Name == svcName {
			found = true
			break
		}
	}
	// Allow "api-gateway-go" (self) as well
	if svcName == "api-gateway-go" {
		found = true
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "service not found in config"})
		return
	}

	var req controlRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "action required (start|stop|restart|status)"})
		return
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	if !allowedActions[action] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid action, must be one of: start, stop, restart, status"})
		return
	}

	// Map config service name to systemd unit name.
	// Convention: service name in config == systemd unit name.
	// e.g., "wetalk-academy" -> systemctl restart wetalk-academy
	unitName := svcName

	start := time.Now()
	cmd := exec.Command("systemctl", action, unitName)
	output, err := cmd.CombinedOutput()
	elapsed := time.Since(start).Milliseconds()

	result := gin.H{
		"service":   svcName,
		"action":    action,
		"output":    strings.TrimSpace(string(output)),
		"elapsed_ms": elapsed,
	}
	if err != nil {
		result["error"] = err.Error()
		c.JSON(http.StatusInternalServerError, result)
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetDBStatus checks Postgres, MongoDB, Redis connectivity.
// GET /gateway/api/db/status
func (h *ServiceControlHandler) GetDBStatus(c *gin.Context) {
	db := h.conf.Database
	statuses := []gateway.DBStatus{
		gateway.CheckPostgres(db.Postgres.DSN),
		gateway.CheckMongo(db.Mongo.URI),
		gateway.CheckRedisDB(db.Redis.URL),
	}
	c.JSON(http.StatusOK, gin.H{"databases": statuses})
}
