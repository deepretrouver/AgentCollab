package realtime

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
)

type Handler struct {
	auth       *auth.Service
	repository Repository
	broker     *Broker
	upgrader   websocket.Upgrader
}

func NewHandler(authService *auth.Service, repository Repository, broker *Broker) *Handler {
	handler := &Handler{auth: authService, repository: repository, broker: broker}
	handler.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 4096,
		CheckOrigin:     sameOrigin,
	}
	return handler
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/events", h.connect)
}

func (h *Handler) connect(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	user, err := h.auth.Authenticate(r.Context(), cookie.Value)
	if err != nil {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	roomID := domain.RoomID(strings.TrimSpace(r.URL.Query().Get("room_id")))
	if roomID == "" {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}
	after, err := strconv.ParseUint(defaultString(r.URL.Query().Get("after"), "0"), 10, 64)
	if err != nil {
		http.Error(w, "after must be an unsigned integer", http.StatusBadRequest)
		return
	}
	if _, err := h.repository.RoomWorkspace(r.Context(), roomID, user.ID); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	connection, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer connection.Close()

	subscription := h.broker.Subscribe(string(roomID))
	defer subscription.Cancel()
	replay, err := LoadReplay(r.Context(), h.repository, roomID, user.ID, after)
	if err != nil {
		_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "replay failed"), time.Now().Add(writeTimeout))
		return
	}
	_ = NewClient(connection).Serve(r.Context(), replay, subscription.Events(), after)
}

func sameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && strings.EqualFold(parsed.Host, r.Host)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
