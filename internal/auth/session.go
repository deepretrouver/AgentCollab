package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/idgen"
)

var (
	ErrInvalidCredentials       = errors.New("invalid credentials")
	ErrUnauthenticated          = errors.New("unauthenticated")
	ErrForbidden                = errors.New("forbidden")
	ErrInviteInvalid            = errors.New("invite is invalid, expired, revoked, or already used")
	ErrConflict                 = errors.New("resource already exists")
	ErrWorkspaceHasActiveTasks  = errors.New("workspace has active tasks")
	ErrLastHubAdmin             = errors.New("last Hub administrator cannot be archived")
	ErrAccountOwnsWorkspace     = errors.New("account owns a workspace")
	ErrAccountHasInfrastructure = errors.New("account has local Agents or devices")
	ErrDefaultHubAdmin          = errors.New("default Hub administrator cannot be removed")
	ErrNotFound                 = errors.New("resource not found")
	ErrInvalidInput             = errors.New("invalid input")
	ErrCoordinatorRequired      = errors.New("room coordinator is required")
	ErrGitMergeRequired         = errors.New("git merge is required")
)

const defaultSessionDuration = 24 * time.Hour
const maxUsernameRunes = 64
const DefaultHubAdminUsername = "owner"

type Session struct {
	ID        string
	UserID    domain.UserID
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
	RevokedAt *time.Time
}

type LoginResult struct {
	User      domain.User
	Token     string
	ExpiresAt time.Time
}

// ProfileInput contains the member-owned information shown to people who
// share a workspace with them. It intentionally contains no role or
// workspace membership fields.
type ProfileInput struct {
	DisplayName  string `json:"display_name"`
	AvatarURL    string `json:"avatar_url"`
	Bio          string `json:"bio"`
	CurrentFocus string `json:"current_focus"`
}

type Repository interface {
	CreateUser(context.Context, domain.User) error
	UserByUsername(context.Context, string) (domain.User, error)
	UserByID(context.Context, domain.UserID) (domain.User, error)
	ListUsers(context.Context) ([]domain.User, error)
	UpdateUserProfile(context.Context, domain.UserID, ProfileInput, time.Time) (domain.User, error)
	UpdateUserRole(context.Context, domain.UserID, domain.UserRole, time.Time) (domain.User, error)
	ArchiveUser(context.Context, domain.UserID, time.Time) error
	CreateSession(context.Context, Session) error
	UserBySessionHash(context.Context, string, time.Time) (domain.User, error)
	RevokeSession(context.Context, string, time.Time) error
	WorkspaceRole(context.Context, domain.WorkspaceID, domain.UserID) (domain.WorkspaceRole, error)
	RoomRole(context.Context, domain.RoomID, domain.UserID) (domain.RoomRole, error)
	CreateInvite(context.Context, Invite) error
	ValidateInvite(context.Context, string, time.Time) error
	ConsumeInvite(context.Context, string, domain.UserID, time.Time) (domain.WorkspaceMembership, error)
	CreateRunnerInvite(context.Context, RunnerInvite) error
}

type Service struct {
	repository      Repository
	now             func() time.Time
	sessionDuration time.Duration
}

func NewService(repository Repository) *Service {
	return &Service{
		repository:      repository,
		now:             func() time.Time { return time.Now().UTC() },
		sessionDuration: defaultSessionDuration,
	}
}

func (s *Service) CreateUser(ctx context.Context, username, displayName, password string, role domain.UserRole) (domain.User, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	displayName = strings.TrimSpace(displayName)
	if err := validateUsername(username); err != nil {
		return domain.User{}, err
	}
	if displayName == "" {
		return domain.User{}, fmt.Errorf("display name is required")
	}
	if role != domain.UserAdmin && role != domain.UserMember {
		return domain.User{}, fmt.Errorf("invalid user role %q", role)
	}
	passwordHash, err := HashPassword(password)
	if err != nil {
		return domain.User{}, err
	}
	id, err := idgen.New("user")
	if err != nil {
		return domain.User{}, err
	}
	now := s.now()
	user := domain.User{
		ID:           domain.UserID(id),
		Username:     username,
		DisplayName:  displayName,
		PasswordHash: passwordHash,
		Role:         role,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repository.CreateUser(ctx, user); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

// validateUsername keeps a login name usable without forcing a technical ASCII
// identifier. Human accounts may use Chinese, other scripts, spaces, emoji, and
// ordinary punctuation; only empty, control-character, and excessive names are
// rejected.
func validateUsername(username string) error {
	if username == "" {
		return fmt.Errorf("用户名不能为空")
	}
	if utf8.RuneCountInString(username) > maxUsernameRunes {
		return fmt.Errorf("用户名最多 64 个字符")
	}
	for _, character := range username {
		if !unicode.IsGraphic(character) {
			return fmt.Errorf("用户名不能包含不可见控制字符")
		}
	}
	return nil
}

// Register creates a regular account for this Hub. It deliberately does not
// grant membership to any existing workspace; a workspace invite remains the
// only way to join someone else's collaboration space.
func (s *Service) Register(ctx context.Context, username, displayName, password string) (LoginResult, error) {
	if _, err := s.CreateUser(ctx, username, displayName, password, domain.UserMember); err != nil {
		if !errors.Is(err, ErrConflict) {
			err = fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		return LoginResult{}, err
	}
	return s.Login(ctx, username, password)
}

func (s *Service) UpdateProfile(ctx context.Context, userID domain.UserID, input ProfileInput) (domain.User, error) {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Bio = strings.TrimSpace(input.Bio)
	input.CurrentFocus = strings.TrimSpace(input.CurrentFocus)
	input.AvatarURL = strings.TrimSpace(input.AvatarURL)
	if input.DisplayName == "" || len([]rune(input.DisplayName)) > 80 || len([]rune(input.Bio)) > 500 || len([]rune(input.CurrentFocus)) > 240 {
		return domain.User{}, ErrInvalidInput
	}
	if input.AvatarURL != "" && (!strings.HasPrefix(input.AvatarURL, "data:image/png;base64,") && !strings.HasPrefix(input.AvatarURL, "data:image/jpeg;base64,") && !strings.HasPrefix(input.AvatarURL, "data:image/webp;base64,")) {
		return domain.User{}, ErrInvalidInput
	}
	// The profile picture is local data, not an external tracking URL. Keep it
	// bounded so a single profile cannot inflate the self-hosted SQLite DB.
	if len(input.AvatarURL) > 1_500_000 {
		return domain.User{}, ErrInvalidInput
	}
	return s.repository.UpdateUserProfile(ctx, userID, input, s.now())
}

// ListHubUsers returns the Hub account directory to a platform administrator.
// It deliberately does not return workspace, room, device, or Agent data.
func (s *Service) ListHubUsers(ctx context.Context, actorID domain.UserID) ([]domain.User, error) {
	actor, err := s.repository.UserByID(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if actor.Role != domain.UserAdmin {
		return nil, ErrForbidden
	}
	return s.repository.ListUsers(ctx)
}

// UpdateHubUserRole manages the Hub-wide account role. Workspace and room
// roles remain separate and are never changed here.
func (s *Service) UpdateHubUserRole(ctx context.Context, actorID, targetID domain.UserID, role domain.UserRole) (domain.User, error) {
	if role != domain.UserAdmin && role != domain.UserMember {
		return domain.User{}, ErrInvalidInput
	}
	actor, err := s.repository.UserByID(ctx, actorID)
	if err != nil {
		return domain.User{}, err
	}
	if actor.Role != domain.UserAdmin || actorID == targetID {
		return domain.User{}, ErrForbidden
	}
	target, err := s.repository.UserByID(ctx, targetID)
	if err != nil {
		return domain.User{}, err
	}
	if target.Username == DefaultHubAdminUsername && role != domain.UserAdmin {
		return domain.User{}, ErrDefaultHubAdmin
	}
	return s.repository.UpdateUserRole(ctx, targetID, role, s.now())
}

// ArchiveHubUser disables an account and removes it from the active Hub
// directory. Historic conversation records retain the original account
// identity. Owners and local Agent/device owners must be cleaned up first so
// archiving can never strand a workspace or a runnable local computer.
func (s *Service) ArchiveHubUser(ctx context.Context, actorID, targetID domain.UserID) error {
	actor, err := s.repository.UserByID(ctx, actorID)
	if err != nil {
		return err
	}
	if actor.Role != domain.UserAdmin || actorID == targetID {
		return ErrForbidden
	}
	target, err := s.repository.UserByID(ctx, targetID)
	if err != nil {
		return err
	}
	if target.Username == DefaultHubAdminUsername {
		return ErrDefaultHubAdmin
	}
	return s.repository.ArchiveUser(ctx, targetID, s.now())
}

// RegisterWithInvite creates a normal human account only when the caller holds
// a currently valid, single-use workspace invitation.
func (s *Service) RegisterWithInvite(ctx context.Context, code, username, displayName, password string) (LoginResult, domain.WorkspaceMembership, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return LoginResult{}, domain.WorkspaceMembership{}, ErrInviteInvalid
	}
	if err := s.repository.ValidateInvite(ctx, HashToken(code), s.now()); err != nil {
		return LoginResult{}, domain.WorkspaceMembership{}, err
	}
	user, err := s.CreateUser(ctx, username, displayName, password, domain.UserMember)
	if err != nil {
		if !errors.Is(err, ErrConflict) {
			err = fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		return LoginResult{}, domain.WorkspaceMembership{}, err
	}
	membership, err := s.AcceptInvite(ctx, code, user.ID)
	if err != nil {
		return LoginResult{}, domain.WorkspaceMembership{}, err
	}
	login, err := s.Login(ctx, username, password)
	if err != nil {
		return LoginResult{}, domain.WorkspaceMembership{}, err
	}
	return login, membership, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (LoginResult, error) {
	user, err := s.repository.UserByUsername(ctx, strings.ToLower(strings.TrimSpace(username)))
	if errors.Is(err, ErrNotFound) {
		return LoginResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return LoginResult{}, err
	}
	if VerifyPassword(user.PasswordHash, password) != nil {
		return LoginResult{}, ErrInvalidCredentials
	}
	token, err := newRawToken(32)
	if err != nil {
		return LoginResult{}, err
	}
	sessionID, err := idgen.New("session")
	if err != nil {
		return LoginResult{}, err
	}
	now := s.now()
	session := Session{
		ID:        sessionID,
		UserID:    user.ID,
		TokenHash: HashToken(token),
		ExpiresAt: now.Add(s.sessionDuration),
		CreatedAt: now,
	}
	if err := s.repository.CreateSession(ctx, session); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{User: user, Token: token, ExpiresAt: session.ExpiresAt}, nil
}

// VerifyCredentials authenticates a non-browser client without creating a
// browser session. The Git smart-HTTP transport uses it for each clone/fetch/
// push request, then applies workspace membership independently.
func (s *Service) VerifyCredentials(ctx context.Context, username, password string) (domain.User, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	user, err := s.repository.UserByUsername(ctx, username)
	if err != nil || VerifyPassword(user.PasswordHash, password) != nil {
		return domain.User{}, ErrInvalidCredentials
	}
	return user, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (domain.User, error) {
	if token == "" {
		return domain.User{}, ErrUnauthenticated
	}
	user, err := s.repository.UserBySessionHash(ctx, HashToken(token), s.now())
	if err != nil {
		return domain.User{}, ErrUnauthenticated
	}
	return user, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return ErrUnauthenticated
	}
	if err := s.repository.RevokeSession(ctx, HashToken(token), s.now()); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrUnauthenticated
		}
		return err
	}
	return nil
}

func newRawToken(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
