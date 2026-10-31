package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/httpapi"
	store "agentcollab.local/agentcollab/internal/store/sqlite"
)

func TestAuthAndInviteHTTPFlow(t *testing.T) {
	database, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "agentcollab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := auth.NewService(store.NewAuthRepository(database))
	owner, err := service.CreateUser(context.Background(), "owner", "Owner", "password-1234", domain.UserAdmin)
	if err != nil {
		t.Fatal(err)
	}
	member, err := service.CreateUser(context.Background(), "member", "Member", "password-5678", domain.UserMember)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO workspaces (id, name, owner_user_id) VALUES ('workspace-1', 'Workspace', ?)`, owner.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ('workspace-1', ?, 'owner')`, owner.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO rooms (id, workspace_id, name) VALUES ('room-1', 'workspace-1', 'Shared room')`); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO room_members (room_id, user_id) VALUES ('room-1', ?)`, owner.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	httpapi.NewAuthHandlersWithRunnerURLs(service, []string{"http://192.168.1.20:8080"}).Register(mux)
	standaloneRegister := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"username":"standalone","display_name":"Standalone","password":"password-0000"}`))
	standaloneResponse := httptest.NewRecorder()
	mux.ServeHTTP(standaloneResponse, standaloneRegister)
	if standaloneResponse.Code != http.StatusCreated || !bytes.Contains(standaloneResponse.Body.Bytes(), []byte(`"username":"standalone"`)) {
		t.Fatalf("POST standalone register status=%d body=%s", standaloneResponse.Code, standaloneResponse.Body.String())
	}
	var standalone struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(standaloneResponse.Body.Bytes(), &standalone); err != nil || standalone.ID == "" {
		t.Fatalf("standalone register response=%q err=%v", standaloneResponse.Body.String(), err)
	}
	standaloneCookie := firstCookie(t, standaloneResponse)
	profileRequest := httptest.NewRequest(http.MethodPatch, "/api/v1/auth/me", bytes.NewBufferString(`{"display_name":"Standalone Research","bio":"Research owner","current_focus":"Testing Hub","avatar_url":"data:image/png;base64,aGVsbG8="}`))
	profileRequest.AddCookie(standaloneCookie)
	profileResponse := httptest.NewRecorder()
	mux.ServeHTTP(profileResponse, profileRequest)
	if profileResponse.Code != http.StatusOK || !bytes.Contains(profileResponse.Body.Bytes(), []byte(`"current_focus":"Testing Hub"`)) {
		t.Fatalf("PATCH profile status=%d body=%s", profileResponse.Code, profileResponse.Body.String())
	}
	ownerCookie := loginCookie(t, mux, "owner", "password-1234")
	memberCookie := loginCookie(t, mux, "member", "password-5678")

	memberAdminListRequest := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	memberAdminListRequest.AddCookie(memberCookie)
	memberAdminListResponse := httptest.NewRecorder()
	mux.ServeHTTP(memberAdminListResponse, memberAdminListRequest)
	if memberAdminListResponse.Code != http.StatusForbidden {
		t.Fatalf("member GET admin users status=%d body=%s", memberAdminListResponse.Code, memberAdminListResponse.Body.String())
	}
	adminListRequest := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	adminListRequest.AddCookie(ownerCookie)
	adminListResponse := httptest.NewRecorder()
	mux.ServeHTTP(adminListResponse, adminListRequest)
	if adminListResponse.Code != http.StatusOK || !bytes.Contains(adminListResponse.Body.Bytes(), []byte(`"username":"member"`)) {
		t.Fatalf("admin GET users status=%d body=%s", adminListResponse.Code, adminListResponse.Body.String())
	}
	promoteRequest := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/"+string(member.ID), bytes.NewBufferString(`{"role":"admin"}`))
	promoteRequest.AddCookie(ownerCookie)
	promoteResponse := httptest.NewRecorder()
	mux.ServeHTTP(promoteResponse, promoteRequest)
	if promoteResponse.Code != http.StatusOK || !bytes.Contains(promoteResponse.Body.Bytes(), []byte(`"role":"admin"`)) {
		t.Fatalf("promote Hub admin status=%d body=%s", promoteResponse.Code, promoteResponse.Body.String())
	}
	promotedAdminListRequest := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	promotedAdminListRequest.AddCookie(memberCookie)
	promotedAdminListResponse := httptest.NewRecorder()
	mux.ServeHTTP(promotedAdminListResponse, promotedAdminListRequest)
	if promotedAdminListResponse.Code != http.StatusOK {
		t.Fatalf("promoted member GET admin users status=%d body=%s", promotedAdminListResponse.Code, promotedAdminListResponse.Body.String())
	}

	archiveRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/"+standalone.ID, nil)
	archiveRequest.AddCookie(ownerCookie)
	archiveResponse := httptest.NewRecorder()
	mux.ServeHTTP(archiveResponse, archiveRequest)
	if archiveResponse.Code != http.StatusNoContent {
		t.Fatalf("DELETE standalone account status=%d body=%s", archiveResponse.Code, archiveResponse.Body.String())
	}
	archivedSessionRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	archivedSessionRequest.AddCookie(standaloneCookie)
	archivedSessionResponse := httptest.NewRecorder()
	mux.ServeHTTP(archivedSessionResponse, archivedSessionRequest)
	if archivedSessionResponse.Code != http.StatusUnauthorized {
		t.Fatalf("archived account session status=%d body=%s", archivedSessionResponse.Code, archivedSessionResponse.Body.String())
	}
	archivedListRequest := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	archivedListRequest.AddCookie(ownerCookie)
	archivedListResponse := httptest.NewRecorder()
	mux.ServeHTTP(archivedListResponse, archivedListRequest)
	if archivedListResponse.Code != http.StatusOK || bytes.Contains(archivedListResponse.Body.Bytes(), []byte(`"username":"standalone"`)) {
		t.Fatalf("archived account active directory status=%d body=%s", archivedListResponse.Code, archivedListResponse.Body.String())
	}
	archivedLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"standalone","password":"password-0000"}`))
	archivedLoginResponse := httptest.NewRecorder()
	mux.ServeHTTP(archivedLoginResponse, archivedLogin)
	if archivedLoginResponse.Code != http.StatusUnauthorized {
		t.Fatalf("archived account login status=%d body=%s", archivedLoginResponse.Code, archivedLoginResponse.Body.String())
	}

	meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meRequest.AddCookie(ownerCookie)
	meResponse := httptest.NewRecorder()
	mux.ServeHTTP(meResponse, meRequest)
	if meResponse.Code != http.StatusOK {
		t.Fatalf("GET /me status = %d body=%s", meResponse.Code, meResponse.Body.String())
	}

	runnerInviteRequest := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/workspace-1/runner-invites", bytes.NewBufferString(`{"expires_in_seconds":1800}`))
	runnerInviteRequest.AddCookie(ownerCookie)
	runnerInviteResponse := httptest.NewRecorder()
	mux.ServeHTTP(runnerInviteResponse, runnerInviteRequest)
	if runnerInviteResponse.Code != http.StatusCreated || !bytes.Contains(runnerInviteResponse.Body.Bytes(), []byte(`"hub_urls":["http://192.168.1.20:8080"]`)) {
		t.Fatalf("POST runner invite status=%d body=%s", runnerInviteResponse.Code, runnerInviteResponse.Body.String())
	}

	inviteBody := bytes.NewBufferString(`{"role":"member","expires_in_seconds":3600}`)
	inviteRequest := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/workspace-1/invites", inviteBody)
	inviteRequest.AddCookie(ownerCookie)
	inviteResponse := httptest.NewRecorder()
	mux.ServeHTTP(inviteResponse, inviteRequest)
	if inviteResponse.Code != http.StatusCreated {
		t.Fatalf("POST invite status = %d body=%s", inviteResponse.Code, inviteResponse.Body.String())
	}
	var created struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(inviteResponse.Body.Bytes(), &created); err != nil || created.Code == "" {
		t.Fatalf("invite response = %q, %v", inviteResponse.Body.String(), err)
	}

	acceptRequest := httptest.NewRequest(http.MethodPost, "/api/v1/invites/"+created.Code+"/accept", nil)
	acceptRequest.AddCookie(memberCookie)
	acceptResponse := httptest.NewRecorder()
	mux.ServeHTTP(acceptResponse, acceptRequest)
	if acceptResponse.Code != http.StatusOK {
		t.Fatalf("POST accept status = %d body=%s", acceptResponse.Code, acceptResponse.Body.String())
	}

	againRequest := httptest.NewRequest(http.MethodPost, "/api/v1/invites/"+created.Code+"/accept", nil)
	againRequest.AddCookie(memberCookie)
	againResponse := httptest.NewRecorder()
	mux.ServeHTTP(againResponse, againRequest)
	if againResponse.Code != http.StatusConflict {
		t.Fatalf("second accept status = %d body=%s", againResponse.Code, againResponse.Body.String())
	}

	humanInviteRequest := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/workspace-1/invites", bytes.NewBufferString(`{"role":"member","room_id":"room-1","expires_in_seconds":3600}`))
	humanInviteRequest.AddCookie(ownerCookie)
	humanInviteResponse := httptest.NewRecorder()
	mux.ServeHTTP(humanInviteResponse, humanInviteRequest)
	if humanInviteResponse.Code != http.StatusCreated {
		t.Fatalf("POST human invite status = %d body=%s", humanInviteResponse.Code, humanInviteResponse.Body.String())
	}
	var humanInvite struct {
		Code   string `json:"code"`
		RoomID string `json:"room_id"`
	}
	if err := json.Unmarshal(humanInviteResponse.Body.Bytes(), &humanInvite); err != nil || humanInvite.Code == "" || humanInvite.RoomID != "room-1" {
		t.Fatalf("human invite response = %q, %v", humanInviteResponse.Body.String(), err)
	}

	registerRequest := httptest.NewRequest(http.MethodPost, "/api/v1/invites/"+humanInvite.Code+"/register", bytes.NewBufferString(`{"username":"new-person","display_name":"New Person","password":"password-9012"}`))
	registerResponse := httptest.NewRecorder()
	mux.ServeHTTP(registerResponse, registerRequest)
	if registerResponse.Code != http.StatusCreated {
		t.Fatalf("POST register with invite status = %d body=%s", registerResponse.Code, registerResponse.Body.String())
	}
	var registered struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(registerResponse.Body.Bytes(), &registered); err != nil || registered.User.ID == "" || registered.WorkspaceID != "workspace-1" {
		t.Fatalf("register response = %q, %v", registerResponse.Body.String(), err)
	}
	var newMemberCookie *http.Cookie
	for _, cookie := range registerResponse.Result().Cookies() {
		if cookie.Name == auth.CookieName {
			newMemberCookie = cookie
		}
	}
	if newMemberCookie == nil {
		t.Fatal("register response missing session cookie")
	}
	var roomMemberships int
	if err := database.Read(context.Background(), func(db *sql.DB) error {
		return db.QueryRow(`SELECT COUNT(*) FROM room_members WHERE room_id = 'room-1' AND user_id = ?`, registered.User.ID).Scan(&roomMemberships)
	}); err != nil || roomMemberships != 1 {
		t.Fatalf("registered room memberships = %d, error = %v", roomMemberships, err)
	}

	memberRunnerRequest := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/workspace-1/runner-invites", bytes.NewBufferString(`{"expires_in_seconds":1800}`))
	memberRunnerRequest.AddCookie(newMemberCookie)
	memberRunnerResponse := httptest.NewRecorder()
	mux.ServeHTTP(memberRunnerResponse, memberRunnerRequest)
	if memberRunnerResponse.Code != http.StatusCreated || !bytes.Contains(memberRunnerResponse.Body.Bytes(), []byte(`"owner_user_id":"`+registered.User.ID+`"`)) {
		t.Fatalf("member runner invite status=%d body=%s", memberRunnerResponse.Code, memberRunnerResponse.Body.String())
	}
}

func firstCookie(t *testing.T, recorder *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	result := recorder.Result()
	defer result.Body.Close()
	for _, cookie := range result.Cookies() {
		if cookie.Name == auth.CookieName {
			return cookie
		}
	}
	t.Fatal("session cookie not found")
	return nil
}

func loginCookie(t *testing.T, handler http.Handler, username, password string) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", response.Code, response.Body.String())
	}
	result := response.Result()
	for _, cookie := range result.Cookies() {
		if cookie.Name == auth.CookieName {
			return cookie
		}
	}
	t.Fatal("login response missing session cookie")
	return nil
}
