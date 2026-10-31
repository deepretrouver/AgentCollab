package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/events"
	"agentcollab.local/agentcollab/internal/httpapi"
	"agentcollab.local/agentcollab/internal/realtime"
	store "agentcollab.local/agentcollab/internal/store/sqlite"
)

func TestWebSocketLiveAndReplay(t *testing.T) {
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	authService := auth.NewService(store.NewAuthRepository(database))
	owner, err := authService.CreateUser(context.Background(), "owner", "Owner", "password-1234", domain.UserAdmin)
	if err != nil {
		t.Fatal(err)
	}
	repository := store.NewCollabRepository(database)
	now := time.Now().UTC()
	workspace := domain.Workspace{ID: "workspace-1", Name: "Workspace", OwnerUserID: owner.ID, CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateWorkspace(context.Background(), workspace, owner.ID); err != nil {
		t.Fatal(err)
	}
	room := domain.Room{ID: "room-1", WorkspaceID: workspace.ID, Name: "General", CreatedAt: now}
	if err := repository.CreateRoom(context.Background(), room, owner.ID); err != nil {
		t.Fatal(err)
	}

	broker := realtime.NewBroker(8)
	handler := httpapi.NewRouter(authService, repository, broker)
	cookie := loginCookie(t, handler, "owner", "password-1234")
	server := httptest.NewServer(handler)
	defer server.Close()

	connection := dialEvents(t, server.URL, room.ID, 0, cookie)
	firstResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/room-1/messages", map[string]any{"body": "first"}, cookie)
	if firstResponse.Code != http.StatusCreated {
		t.Fatalf("first message status=%d body=%s", firstResponse.Code, firstResponse.Body.String())
	}
	var first events.Envelope
	if err := connection.ReadJSON(&first); err != nil {
		t.Fatal(err)
	}
	if first.Type != "message.created" || first.Sequence != 1 || first.RoomID != "room-1" {
		t.Fatalf("first event=%#v", first)
	}
	_ = connection.Close()

	secondResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/rooms/room-1/messages", map[string]any{"body": "second"}, cookie)
	if secondResponse.Code != http.StatusCreated {
		t.Fatalf("second message status=%d body=%s", secondResponse.Code, secondResponse.Body.String())
	}
	reconnected := dialEvents(t, server.URL, room.ID, 1, cookie)
	defer reconnected.Close()
	var replayed events.Envelope
	if err := reconnected.ReadJSON(&replayed); err != nil {
		t.Fatal(err)
	}
	if replayed.Sequence != 2 || replayed.Type != "message.created" {
		t.Fatalf("replayed event=%#v", replayed)
	}
}

func dialEvents(t *testing.T, serverURL string, roomID domain.RoomID, after uint64, cookie *http.Cookie) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(serverURL, "http") + "/api/v1/events?room_id=" + string(roomID) + "&after=" + strconv.FormatUint(after, 10)
	header := http.Header{}
	header.Set("Cookie", cookie.Name+"="+cookie.Value)
	connection, response, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		if response != nil {
			t.Fatalf("websocket dial: %v status=%d", err, response.StatusCode)
		}
		t.Fatal(err)
	}
	return connection
}
