package webdav

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"MrRSS/internal/handlers/core"
	"MrRSS/internal/webdavsync"
)

func HandleSync(h *core.Handler, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	service := webdavsync.NewService(h.DB, h.Fetcher)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	result, err := service.SyncSubscriptions(ctx)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "success",
		"imported_count": result.ImportedCount,
		"exported_count": result.ExportedCount,
		"remote_exists":  result.RemoteExists,
		"last_sync_time": result.LastSyncTime,
	})
}

func writeJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}
