package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/domain"
	"agentcollab.local/agentcollab/internal/idgen"
)

func (r *CollabRepository) EnrollRunner(ctx context.Context, ownerID domain.UserID, workspaceID domain.WorkspaceID, tokenHash, deviceName, stableKey, osName, arch, version string, registrations []domain.AgentRegistration) (domain.RunnerEnrollment, error) {
	var result domain.RunnerEnrollment
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := workspaceRoleTx(ctx, tx, workspaceID, ownerID); err != nil {
			return err
		}
		var err error
		result, err = enrollRunnerTx(ctx, tx, ownerID, workspaceID, tokenHash, deviceName, stableKey, osName, arch, version, registrations)
		return err
	})
	return result, err
}

// EnrollRunnerWithInvite consumes a one-time invite and creates the device,
// Runner, and Agent identities in the same transaction. A failed enrollment
// never burns the invite, and concurrent use can only succeed once.
func (r *CollabRepository) EnrollRunnerWithInvite(ctx context.Context, inviteCodeHash, tokenHash, deviceName, stableKey, osName, arch, version string, registrations []domain.AgentRegistration) (domain.RunnerEnrollment, error) {
	var result domain.RunnerEnrollment
	now := time.Now().UTC()
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		var workspaceID domain.WorkspaceID
		var ownerID domain.UserID
		if err := tx.QueryRowContext(ctx, `
SELECT workspace_id, owner_user_id FROM runner_invites
WHERE code_hash = ? AND used_at IS NULL AND revoked_at IS NULL AND expires_at > ?`, inviteCodeHash, formatTime(now)).Scan(&workspaceID, &ownerID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return auth.ErrInviteInvalid
			}
			return err
		}
		var err error
		result, err = enrollRunnerTx(ctx, tx, ownerID, workspaceID, tokenHash, deviceName, stableKey, osName, arch, version, registrations)
		if err != nil {
			return err
		}
		updated, err := tx.ExecContext(ctx, `
UPDATE runner_invites SET used_by_runner_id = ?, used_at = ?
WHERE code_hash = ? AND used_at IS NULL AND revoked_at IS NULL`, result.Runner.ID, formatTime(now), inviteCodeHash)
		if err != nil {
			return err
		}
		if count, _ := updated.RowsAffected(); count != 1 {
			return auth.ErrInviteInvalid
		}
		return nil
	})
	return result, err
}

func enrollRunnerTx(ctx context.Context, tx *sql.Tx, ownerID domain.UserID, workspaceID domain.WorkspaceID, tokenHash, deviceName, stableKey, osName, arch, version string, registrations []domain.AgentRegistration) (domain.RunnerEnrollment, error) {
	var result domain.RunnerEnrollment
	now := time.Now().UTC()

	var createdAt string
	lookup := `
SELECT id, created_at FROM devices
WHERE workspace_id = ? AND owner_user_id = ? AND name = ? AND status != 'revoked'
ORDER BY created_at DESC LIMIT 1`
	args := []any{workspaceID, ownerID, deviceName}
	if stableKey != "" {
		lookup = `SELECT id, created_at FROM devices WHERE workspace_id = ? AND owner_user_id = ? AND stable_key = ? AND status != 'revoked' ORDER BY created_at DESC LIMIT 1`
		args = []any{workspaceID, ownerID, stableKey}
	}
	err := tx.QueryRowContext(ctx, lookup, args...).Scan(&result.Device.ID, &createdAt)
	if errors.Is(err, sql.ErrNoRows) && stableKey != "" {
		// Adopt a device enrolled by an older Runner that did not yet send a
		// stable installation key. This avoids creating a duplicate on upgrade.
		err = tx.QueryRowContext(ctx, `
SELECT id, created_at FROM devices
WHERE workspace_id = ? AND owner_user_id = ? AND name = ? AND stable_key = '' AND status != 'revoked'
ORDER BY created_at DESC LIMIT 1`, workspaceID, ownerID, deviceName).Scan(&result.Device.ID, &createdAt)
	}
	if errors.Is(err, sql.ErrNoRows) {
		deviceID, idErr := idgen.New("device")
		if idErr != nil {
			return result, idErr
		}
		result.Device.ID = domain.DeviceID(deviceID)
		result.Device.CreatedAt = now
		if _, err := tx.ExecContext(ctx, `
INSERT INTO devices (id, workspace_id, owner_user_id, name, stable_key, os, arch, status, last_seen_at, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, 'online', ?, ?)`, result.Device.ID, workspaceID, ownerID, deviceName, stableKey, osName, arch, formatTime(now), formatTime(now)); err != nil {
			return result, err
		}
	} else if err != nil {
		return result, err
	} else {
		parsed, parseErr := time.Parse(time.RFC3339Nano, createdAt)
		if parseErr != nil {
			return result, parseErr
		}
		result.Device.CreatedAt = parsed
		if _, err := tx.ExecContext(ctx, `UPDATE devices SET name = ?, stable_key = CASE WHEN ? != '' THEN ? ELSE stable_key END, os = ?, arch = ?, status = 'online', last_seen_at = ? WHERE id = ?`, deviceName, stableKey, stableKey, osName, arch, formatTime(now), result.Device.ID); err != nil {
			return result, err
		}
	}
	result.Device.WorkspaceID = workspaceID
	result.Device.OwnerUserID = ownerID
	result.Device.Name = deviceName
	result.Device.StableKey = stableKey
	result.Device.OS = osName
	result.Device.Arch = arch
	result.Device.Status = domain.DeviceOnline
	result.Device.LastSeenAt = &now

	var runnerCreatedAt string
	err = tx.QueryRowContext(ctx, `SELECT id, created_at FROM runners WHERE device_id = ?`, result.Device.ID).Scan(&result.Runner.ID, &runnerCreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		runnerID, idErr := idgen.New("runner")
		if idErr != nil {
			return result, idErr
		}
		result.Runner.ID = domain.RunnerID(runnerID)
		result.Runner.CreatedAt = now
		if _, err := tx.ExecContext(ctx, `
INSERT INTO runners (id, device_id, token_hash, version, status, last_seen_at, created_at)
VALUES (?, ?, ?, ?, 'online', ?, ?)`, result.Runner.ID, result.Device.ID, tokenHash, version, formatTime(now), formatTime(now)); err != nil {
			return result, err
		}
	} else if err != nil {
		return result, err
	} else {
		parsed, parseErr := time.Parse(time.RFC3339Nano, runnerCreatedAt)
		if parseErr != nil {
			return result, parseErr
		}
		result.Runner.CreatedAt = parsed
		if _, err := tx.ExecContext(ctx, `UPDATE runners SET token_hash = ?, version = ?, status = 'online', last_seen_at = ? WHERE id = ?`, tokenHash, version, formatTime(now), result.Runner.ID); err != nil {
			return result, err
		}
	}
	result.Runner.DeviceID = result.Device.ID
	result.Runner.Version = version
	result.Runner.Status = domain.DeviceOnline
	result.Runner.LastSeenAt = &now

	if _, err := tx.ExecContext(ctx, `UPDATE agents SET status = 'offline', updated_at = ? WHERE runner_id = ?`, formatTime(now), result.Runner.ID); err != nil {
		return result, err
	}
	result.Agents = make([]domain.EnrolledAgent, 0, len(registrations))
	var ownerName, ownerUsername string
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(NULLIF(display_name, ''), username), username FROM users WHERE id = ?`, ownerID).Scan(&ownerName, &ownerUsername); err != nil {
		return result, err
	}
	for _, registration := range registrations {
		capabilities := registration.Capabilities
		if len(capabilities) == 0 || !json.Valid(capabilities) {
			capabilities = json.RawMessage(`{}`)
		}
		var agentID domain.AgentID
		status := domain.AgentUnavailable
		if registration.Available {
			status = domain.AgentOnline
		}
		originalName := registration.DisplayName
		managedName := managedRunnerAgentName(registration.LocalID, registration.DisplayName, registration.Type, ownerUsername, ownerName)
		if managedName {
			registration.DisplayName = ownerAgentDisplayName(ownerName, registration.Type)
		}
		query := `SELECT id FROM agents WHERE runner_id = ? AND workspace_id = ? AND local_key = ?`
		queryArgs := []any{result.Runner.ID, workspaceID, registration.LocalID}
		err := tx.QueryRowContext(ctx, query, queryArgs...).Scan(&agentID)
		if errors.Is(err, sql.ErrNoRows) {
			// Adopt identities written before local_key existed. This keeps Agent
			// history stable across a Runner upgrade and display-name changes.
			query = `SELECT id FROM agents WHERE runner_id = ? AND workspace_id = ? AND display_name = ? AND local_key = ''`
			queryArgs = []any{result.Runner.ID, workspaceID, originalName}
			if managedName {
				query = `SELECT id FROM agents WHERE runner_id = ? AND workspace_id = ? AND type = ? AND local_key = '' ORDER BY created_at LIMIT 1`
				queryArgs = []any{result.Runner.ID, workspaceID, registration.Type}
			}
			err = tx.QueryRowContext(ctx, query, queryArgs...).Scan(&agentID)
		}
		if errors.Is(err, sql.ErrNoRows) {
			generated, idErr := idgen.New("agent")
			if idErr != nil {
				return result, idErr
			}
			agentID = domain.AgentID(generated)
			if _, err := tx.ExecContext(ctx, `
			INSERT INTO agents (id, workspace_id, runner_id, owner_user_id, local_key, runtime_local_key, source_display_name, type, display_name, status, capabilities_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, agentID, workspaceID, result.Runner.ID, ownerID, registration.LocalID, registration.LocalID, registration.DisplayName, registration.Type, registration.DisplayName, status, string(capabilities), formatTime(now), formatTime(now)); err != nil {
				return result, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO agent_access_policies (agent_id, updated_by_user_id) VALUES (?, ?)`, agentID, ownerID); err != nil {
				return result, err
			}
		} else if err != nil {
			return result, err
		} else if _, err := tx.ExecContext(ctx, `
UPDATE agents SET local_key = ?, runtime_local_key = ?, source_display_name = ?, type = ?, display_name = ?, status = ?, capabilities_json = ?, updated_at = ? WHERE id = ?`, registration.LocalID, registration.LocalID, registration.DisplayName, registration.Type, registration.DisplayName, status, string(capabilities), formatTime(now), agentID); err != nil {
			return result, err
		}
		// Workspace reuses are separate Agent records for access-control and
		// conversation history, but they must reflect the same local CLI catalog.
		if _, err := tx.ExecContext(ctx, `
UPDATE agents
SET source_display_name = ?, display_name = ?, type = ?, status = ?, capabilities_json = ?, updated_at = ?
WHERE runner_id = ? AND runtime_local_key = ? AND id != ?`, registration.DisplayName, registration.DisplayName, registration.Type, status, string(capabilities), formatTime(now), result.Runner.ID, registration.LocalID, agentID); err != nil {
			return result, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO agent_access_policies (agent_id, updated_by_user_id) VALUES (?, ?) ON CONFLICT(agent_id) DO NOTHING`, agentID, ownerID); err != nil {
			return result, err
		}
		result.Agents = append(result.Agents, domain.EnrolledAgent{LocalID: registration.LocalID, AgentID: agentID})
	}
	return result, nil
}

func defaultRunnerAgentName(localID, displayName string) bool {
	return strings.HasPrefix(localID, "scan-") || strings.HasSuffix(strings.TrimSpace(displayName), " Local")
}

func managedRunnerAgentName(localID, displayName, agentType string, ownerNames ...string) bool {
	if defaultRunnerAgentName(localID, displayName) {
		return true
	}
	name := strings.TrimSpace(displayName)
	for _, ownerName := range ownerNames {
		if name == ownerAgentDisplayName(ownerName, agentType) {
			return true
		}
	}
	return false
}

func ownerAgentDisplayName(ownerName, agentType string) string {
	brand := map[string]string{
		"hermes":          "Hermes",
		"codex":           "Codex",
		"workbuddy":       "WorkBuddy",
		"claude-code":     "Claude Code",
		"opencode":        "OpenCode",
		"openclaw":        "OpenClaw",
		"codex-acp":       "Codex ACP",
		"claude-code-acp": "Claude Code ACP",
	}[agentType]
	if brand == "" {
		brand = agentType + " Agent"
	}
	return strings.TrimSpace(ownerName) + " " + brand
}

func refreshManagedAgentDisplayNamesTx(ctx context.Context, tx *sql.Tx, ownerID domain.UserID, ownerName string) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, type FROM agents WHERE owner_user_id = ? AND local_key LIKE 'scan-%'`, ownerID)
	if err != nil {
		return err
	}
	type agentNameTarget struct {
		id        domain.AgentID
		agentType string
	}
	var targets []agentNameTarget
	for rows.Next() {
		var target agentNameTarget
		if err := rows.Scan(&target.id, &target.agentType); err != nil {
			rows.Close()
			return err
		}
		targets = append(targets, target)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, target := range targets {
		name := ownerAgentDisplayName(ownerName, target.agentType)
		if _, err := tx.ExecContext(ctx, `UPDATE agents SET source_display_name = ?, display_name = ?, updated_at = ? WHERE id = ?`, name, name, formatTime(time.Now().UTC()), target.id); err != nil {
			return err
		}
	}
	return nil
}

func (r *CollabRepository) AuthenticateRunner(ctx context.Context, tokenHash string) (domain.RunnerIdentity, error) {
	var identity domain.RunnerIdentity
	err := r.store.Read(ctx, func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `
SELECT r.id, d.id, d.workspace_id, d.owner_user_id, d.status
FROM runners r JOIN devices d ON d.id = r.device_id
WHERE r.token_hash = ? AND r.status != 'revoked' AND d.status != 'revoked'`, tokenHash).Scan(
			&identity.RunnerID, &identity.DeviceID, &identity.WorkspaceID, &identity.OwnerUserID, &identity.DeviceStatus,
		)
	})
	return identity, mapNotFound(err)
}

func (r *CollabRepository) RunnerOwnedByUser(ctx context.Context, runnerID domain.RunnerID, userID domain.UserID) (bool, error) {
	var ownerID domain.UserID
	err := r.store.Read(ctx, func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `
SELECT d.owner_user_id
FROM runners r JOIN devices d ON d.id = r.device_id
WHERE r.id = ? AND r.status != 'revoked' AND d.status != 'revoked'`, runnerID).Scan(&ownerID)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return ownerID == userID, nil
}

func (r *CollabRepository) HeartbeatRunner(ctx context.Context, runnerID domain.RunnerID, version, stableKey string, onlineAgentIDs []domain.AgentID, onlineAgentTypes []string, registrations []domain.AgentRegistration) ([]domain.EnrolledAgent, error) {
	now := time.Now().UTC()
	agents := make([]domain.EnrolledAgent, 0, len(registrations))
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE runners SET version = ?, status = 'online', last_seen_at = ? WHERE id = ? AND status != 'revoked'`, version, formatTime(now), runnerID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return auth.ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `UPDATE devices SET stable_key = CASE WHEN ? != '' THEN ? ELSE stable_key END, status = 'online', last_seen_at = ? WHERE id = (SELECT device_id FROM runners WHERE id = ?)`, stableKey, stableKey, formatTime(now), runnerID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE agents SET status = 'offline', updated_at = ? WHERE runner_id = ? AND status != 'unavailable'`, formatTime(now), runnerID); err != nil {
			return err
		}
		matchedReportedAgent := false
		for _, agentID := range onlineAgentIDs {
			var runtimeLocalKey string
			if err := tx.QueryRowContext(ctx, `SELECT COALESCE(NULLIF(runtime_local_key, ''), local_key) FROM agents WHERE id = ? AND runner_id = ?`, agentID, runnerID).Scan(&runtimeLocalKey); err != nil {
				if !errors.Is(err, sql.ErrNoRows) {
					return err
				}
				if err := tx.QueryRowContext(ctx, `SELECT runtime_local_key FROM runner_agent_aliases WHERE runner_id = ? AND legacy_agent_id = ?`, runnerID, agentID).Scan(&runtimeLocalKey); err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						continue
					}
					return err
				}
			}
			matchedReportedAgent = true
			if _, err := tx.ExecContext(ctx, `UPDATE agents SET status = 'online', updated_at = ? WHERE runner_id = ? AND COALESCE(NULLIF(runtime_local_key, ''), local_key) = ? AND status != 'unavailable'`, formatTime(now), runnerID, runtimeLocalKey); err != nil {
				return err
			}
		}
		// Agent records are copied into other workspaces so each workspace can own
		// its own access policy and room history. A copied record gets a new server
		// ID, while a paired Runner can still hold the old ID. The Runner also sends
		// the types it can actually execute; use that stable local fact to restore
		// copied records after a workspace migration without asking the owner to
		// pair the same computer again.
		seenTypes := make(map[string]bool)
		for _, agentType := range onlineAgentTypes {
			agentType = strings.TrimSpace(agentType)
			if agentType == "" || seenTypes[agentType] {
				continue
			}
			seenTypes[agentType] = true
			if _, err := tx.ExecContext(ctx, `UPDATE agents SET status = 'online', updated_at = ? WHERE runner_id = ? AND type = ? AND status != 'unavailable'`, formatTime(now), runnerID, agentType); err != nil {
				return err
			}
		}
		// Old Clients only know their historical server Agent IDs. If every
		// reported ID has already been deleted and no precise type information is
		// available, the Runner itself is still a reliable signal that its existing
		// executable catalog is alive. Restore non-disabled records rather than
		// stranding a collaborator until they update or re-pair their Client.
		if len(onlineAgentIDs) > 0 && !matchedReportedAgent && len(seenTypes) == 0 {
			if _, err := tx.ExecContext(ctx, `UPDATE agents SET status = 'online', updated_at = ? WHERE runner_id = ? AND status != 'unavailable'`, formatTime(now), runnerID); err != nil {
				return err
			}
		}
		if len(registrations) > 0 {
			agents, err = syncRunnerCatalogTx(ctx, tx, runnerID, registrations, now)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return agents, err
}

// syncRunnerCatalogTx reconciles the Runner's current local catalog without
// changing its token or asking the owner to pair the same computer again. The
// device workspace owns any newly discovered source record; copies already
// imported into other workspaces keep their independent access policy while
// receiving the same executable status and capabilities.
func syncRunnerCatalogTx(ctx context.Context, tx *sql.Tx, runnerID domain.RunnerID, registrations []domain.AgentRegistration, now time.Time) ([]domain.EnrolledAgent, error) {
	var workspaceID domain.WorkspaceID
	var ownerID domain.UserID
	var ownerName, ownerUsername string
	if err := tx.QueryRowContext(ctx, `
SELECT d.workspace_id, d.owner_user_id, COALESCE(NULLIF(u.display_name, ''), u.username), u.username
FROM runners r
JOIN devices d ON d.id = r.device_id
JOIN users u ON u.id = d.owner_user_id
WHERE r.id = ? AND r.status != 'revoked' AND d.status != 'revoked'`, runnerID).Scan(&workspaceID, &ownerID, &ownerName, &ownerUsername); err != nil {
		return nil, mapNotFound(err)
	}

	result := make([]domain.EnrolledAgent, 0, len(registrations))
	seenLocalIDs := make(map[string]bool, len(registrations))
	for _, registration := range registrations {
		if seenLocalIDs[registration.LocalID] {
			continue
		}
		seenLocalIDs[registration.LocalID] = true
		capabilities := registration.Capabilities
		if len(capabilities) == 0 || !json.Valid(capabilities) {
			capabilities = json.RawMessage(`{}`)
		}
		status := domain.AgentUnavailable
		if registration.Available {
			status = domain.AgentOnline
		}
		sourceDisplayName := registration.DisplayName
		displayName := registration.DisplayName
		if managedRunnerAgentName(registration.LocalID, registration.DisplayName, registration.Type, ownerUsername, ownerName) {
			displayName = ownerAgentDisplayName(ownerName, registration.Type)
			sourceDisplayName = displayName
		}

		var agentID domain.AgentID
		err := tx.QueryRowContext(ctx, `
SELECT id FROM agents
WHERE runner_id = ? AND workspace_id = ?
  AND COALESCE(NULLIF(runtime_local_key, ''), local_key) = ?
ORDER BY created_at, id LIMIT 1`, runnerID, workspaceID, registration.LocalID).Scan(&agentID)
		if errors.Is(err, sql.ErrNoRows) {
			// A configured ID may replace an older scan-* ID after an upgrade.
			// If this Runner has exactly one real local identity of the same
			// Agent type, inherit that identity instead of showing a duplicate
			// candidate. Multiple local profiles stay deliberately unresolved.
			legacyRuntimeKey, legacyAgentID, adoptErr := uniqueRunnerTypeIdentityTx(ctx, tx, runnerID, workspaceID, registration.Type)
			if adoptErr != nil {
				return nil, adoptErr
			}
			if legacyAgentID != "" {
				agentID = legacyAgentID
				if _, err := tx.ExecContext(ctx, `
UPDATE agents SET runtime_local_key = ?, updated_at = ?
WHERE runner_id = ? AND COALESCE(NULLIF(runtime_local_key, ''), local_key) = ?`,
					registration.LocalID, formatTime(now), runnerID, legacyRuntimeKey); err != nil {
					return nil, err
				}
				if _, err := tx.ExecContext(ctx, `
UPDATE runner_agent_aliases SET runtime_local_key = ?
WHERE runner_id = ? AND runtime_local_key = ?`,
					registration.LocalID, runnerID, legacyRuntimeKey); err != nil {
					return nil, err
				}
			} else {
				generated, idErr := idgen.New("agent")
				if idErr != nil {
					return nil, idErr
				}
				agentID = domain.AgentID(generated)
				if _, err := tx.ExecContext(ctx, `
INSERT INTO agents (
  id, workspace_id, runner_id, owner_user_id, local_key, runtime_local_key,
  source_display_name, type, display_name, status, capabilities_json,
  imported, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
					agentID, workspaceID, runnerID, ownerID, registration.LocalID,
					registration.LocalID, sourceDisplayName, registration.Type,
					displayName, status, string(capabilities), formatTime(now),
					formatTime(now)); err != nil {
					return nil, err
				}
			}
		} else if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE agents
SET runtime_local_key = ?, source_display_name = ?, display_name = ?, type = ?, status = ?,
    capabilities_json = ?, updated_at = ?
WHERE id = ?`, registration.LocalID, sourceDisplayName, displayName, registration.Type,
			status, string(capabilities), formatTime(now), agentID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE agents
SET source_display_name = ?, display_name = ?, type = ?, status = ?, capabilities_json = ?,
    updated_at = ?
WHERE runner_id = ? AND runtime_local_key = ? AND id != ?`,
			sourceDisplayName, displayName, registration.Type, status, string(capabilities),
			formatTime(now), runnerID, registration.LocalID, agentID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO agent_access_policies (agent_id, updated_by_user_id)
VALUES (?, ?) ON CONFLICT(agent_id) DO NOTHING`, agentID, ownerID); err != nil {
			return nil, err
		}
		result = append(result, domain.EnrolledAgent{LocalID: registration.LocalID, AgentID: agentID})
	}
	return result, nil
}

func uniqueRunnerTypeIdentityTx(ctx context.Context, tx *sql.Tx, runnerID domain.RunnerID, workspaceID domain.WorkspaceID, agentType string) (string, domain.AgentID, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT COALESCE(NULLIF(runtime_local_key, ''), local_key)
FROM agents
WHERE runner_id = ? AND type = ?
  AND COALESCE(NULLIF(runtime_local_key, ''), local_key) != ''
GROUP BY COALESCE(NULLIF(runtime_local_key, ''), local_key)
ORDER BY MIN(created_at), MIN(id)
LIMIT 2`, runnerID, agentType)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()
	keys := make([]string, 0, 2)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return "", "", err
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return "", "", err
	}
	if len(keys) != 1 {
		return "", "", nil
	}
	var agentID domain.AgentID
	if err := tx.QueryRowContext(ctx, `
SELECT id FROM agents
WHERE runner_id = ?
  AND COALESCE(NULLIF(runtime_local_key, ''), local_key) = ?
ORDER BY workspace_id = ? DESC, created_at, id
LIMIT 1`, runnerID, keys[0], workspaceID).Scan(&agentID); err != nil {
		return "", "", mapNotFound(err)
	}
	return keys[0], agentID, nil
}

func (r *CollabRepository) ListAgents(ctx context.Context, workspaceID domain.WorkspaceID, userID domain.UserID) ([]domain.AgentIdentity, error) {
	return r.listAgents(ctx, workspaceID, userID, true)
}

func (r *CollabRepository) ListAgentCandidates(ctx context.Context, workspaceID domain.WorkspaceID, userID domain.UserID) ([]domain.AgentIdentity, error) {
	return r.listAgents(ctx, workspaceID, userID, false)
}

const runnerOnlineWindow = 30 * time.Second

func (r *CollabRepository) listAgents(ctx context.Context, workspaceID domain.WorkspaceID, userID domain.UserID, importedOnly bool) ([]domain.AgentIdentity, error) {
	agents := make([]domain.AgentIdentity, 0)
	err := r.store.Read(ctx, func(db *sql.DB) error {
		var role domain.WorkspaceRole
		if err := db.QueryRowContext(ctx, `SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspaceID, userID).Scan(&role); err != nil {
			return mapNotFound(err)
		}
		query := `
SELECT a.id, COALESCE(NULLIF(a.source_display_name, ''), a.display_name), a.type, a.owner_user_id, u.display_name, d.id, d.name, d.os, d.arch,
       'runner/' || lower(d.os), COALESCE(json_extract(a.capabilities_json, '$.permission_mode'), 'restricted'),
	       CASE
	         WHEN r.status != 'online' OR r.last_seen_at < ? THEN 'offline'
	         WHEN a.status = 'unavailable' THEN 'unavailable'
	         WHEN EXISTS (
	           SELECT 1 FROM tasks active JOIN agents task_agent ON task_agent.id = active.target_agent_id
	           WHERE task_agent.runner_id = a.runner_id
	             AND COALESCE(NULLIF(task_agent.runtime_local_key, ''), task_agent.local_key) = COALESCE(NULLIF(a.runtime_local_key, ''), a.local_key)
	             AND active.state IN ('dispatched', 'running')
	         ) THEN 'busy'
	         ELSE a.status
	       END,
	       a.imported, COALESCE(json_extract(a.capabilities_json, '$.execution'), 0),
	       COALESCE(json_extract(a.capabilities_json, '$.version'), ''), COALESCE(json_extract(a.capabilities_json, '$.executable'), ''),
		       COALESCE(json_extract(a.capabilities_json, '$.native_history'), ''), a.capabilities_json,
		       r.version, r.last_seen_at,
		       (SELECT COUNT(*) FROM tasks queued JOIN agents task_agent ON task_agent.id = queued.target_agent_id WHERE task_agent.runner_id = a.runner_id AND COALESCE(NULLIF(task_agent.runtime_local_key, ''), task_agent.local_key) = COALESCE(NULLIF(a.runtime_local_key, ''), a.local_key) AND queued.state = 'queued'),
		       (SELECT COUNT(*) FROM tasks active JOIN agents task_agent ON task_agent.id = active.target_agent_id WHERE task_agent.runner_id = a.runner_id AND COALESCE(NULLIF(task_agent.runtime_local_key, ''), task_agent.local_key) = COALESCE(NULLIF(a.runtime_local_key, ''), a.local_key) AND active.state IN ('dispatched', 'running')),
		       EXISTS (SELECT 1 FROM agent_sessions session WHERE session.agent_id = a.id AND session.state = 'active'),
		       p.visibility, p.invocation_scope, p.shared_skills_json,
		       COALESCE(NULLIF(a.runtime_local_key, ''), a.local_key)
FROM agents a
JOIN runners r ON r.id = a.runner_id
JOIN devices d ON d.id = r.device_id
JOIN users u ON u.id = a.owner_user_id
JOIN agent_access_policies p ON p.agent_id = a.id
	WHERE a.workspace_id = ? AND p.archived = 0 AND a.imported = 1
	  AND (p.visibility = 'workspace' OR a.owner_user_id = ?)
	ORDER BY a.owner_user_id = ? DESC, a.created_at, a.id`
		args := []any{formatTime(time.Now().UTC().Add(-runnerOnlineWindow)), workspaceID, userID, userID}
		if !importedOnly {
			query = `
SELECT a.id, COALESCE(NULLIF(a.source_display_name, ''), a.display_name), a.type, a.owner_user_id, u.display_name, d.id, d.name, d.os, d.arch,
       'runner/' || lower(d.os), COALESCE(json_extract(a.capabilities_json, '$.permission_mode'), 'restricted'),
	       CASE
	         WHEN r.status != 'online' OR r.last_seen_at < ? THEN 'offline'
	         WHEN a.status = 'unavailable' THEN 'unavailable'
	         WHEN EXISTS (
	           SELECT 1 FROM tasks active JOIN agents task_agent ON task_agent.id = active.target_agent_id
	           WHERE task_agent.runner_id = a.runner_id
	             AND COALESCE(NULLIF(task_agent.runtime_local_key, ''), task_agent.local_key) = COALESCE(NULLIF(a.runtime_local_key, ''), a.local_key)
	             AND active.state IN ('dispatched', 'running')
	         ) THEN 'busy'
	         ELSE a.status
	       END,
       a.imported, COALESCE(json_extract(a.capabilities_json, '$.execution'), 0),
       COALESCE(json_extract(a.capabilities_json, '$.version'), ''), COALESCE(json_extract(a.capabilities_json, '$.executable'), ''),
       COALESCE(json_extract(a.capabilities_json, '$.native_history'), ''), a.capabilities_json,
		       r.version, r.last_seen_at,
		       (SELECT COUNT(*) FROM tasks queued JOIN agents task_agent ON task_agent.id = queued.target_agent_id WHERE task_agent.runner_id = a.runner_id AND COALESCE(NULLIF(task_agent.runtime_local_key, ''), task_agent.local_key) = COALESCE(NULLIF(a.runtime_local_key, ''), a.local_key) AND queued.state = 'queued'),
		       (SELECT COUNT(*) FROM tasks active JOIN agents task_agent ON task_agent.id = active.target_agent_id WHERE task_agent.runner_id = a.runner_id AND COALESCE(NULLIF(task_agent.runtime_local_key, ''), task_agent.local_key) = COALESCE(NULLIF(a.runtime_local_key, ''), a.local_key) AND active.state IN ('dispatched', 'running')),
		       EXISTS (SELECT 1 FROM agent_sessions session WHERE session.agent_id = a.id AND session.state = 'active'),
		       p.visibility, p.invocation_scope, p.shared_skills_json,
       COALESCE(NULLIF(a.runtime_local_key, ''), a.local_key)
FROM agents a
JOIN runners r ON r.id = a.runner_id
JOIN devices d ON d.id = r.device_id
JOIN users u ON u.id = a.owner_user_id
JOIN agent_access_policies p ON p.agent_id = a.id
WHERE a.owner_user_id = ? AND p.archived = 0
  AND r.status = 'online' AND d.status = 'online' AND r.last_seen_at >= ?
  AND a.status = 'online' AND COALESCE(json_extract(a.capabilities_json, '$.execution'), 0) = 1
  AND (
    (a.workspace_id = ? AND a.imported = 0)
    OR (a.workspace_id != ? AND NOT EXISTS (
      SELECT 1 FROM agents current
      WHERE current.workspace_id = ? AND current.runner_id = a.runner_id
        AND COALESCE(NULLIF(current.runtime_local_key, ''), current.local_key) = COALESCE(NULLIF(a.runtime_local_key, ''), a.local_key)
    ))
  )
ORDER BY a.workspace_id = ? DESC, a.created_at, a.id`
			args = []any{formatTime(time.Now().UTC().Add(-runnerOnlineWindow)), userID, formatTime(time.Now().UTC().Add(-runnerOnlineWindow)), workspaceID, workspaceID, workspaceID, workspaceID}
		}
		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		seenRuntimeKeys := make(map[string]bool)
		for rows.Next() {
			var agent domain.AgentIdentity
			var capabilitiesJSON, sharedSkillsJSON, runtimeLocalKey string
			var runnerLastSeenAt sql.NullString
			if err := rows.Scan(&agent.AgentID, &agent.DisplayName, &agent.Type, &agent.OwnerUserID, &agent.OwnerName, &agent.DeviceID, &agent.DeviceName, &agent.OS, &agent.Arch, &agent.Channel, &agent.PermissionMode, &agent.Status, &agent.Imported, &agent.Available, &agent.Version, &agent.Executable, &agent.NativeHistory, &capabilitiesJSON, &agent.RunnerVersion, &runnerLastSeenAt, &agent.QueuedTaskCount, &agent.RunningTaskCount, &agent.NativeSessionReady, &agent.Visibility, &agent.InvocationScope, &sharedSkillsJSON, &runtimeLocalKey); err != nil {
				return err
			}
			if runnerLastSeenAt.Valid {
				if parsed, parseErr := time.Parse(time.RFC3339Nano, runnerLastSeenAt.String); parseErr == nil {
					agent.RunnerLastSeenAt = &parsed
				}
			}
			if !importedOnly {
				key := string(agent.DeviceID) + "\x00" + runtimeLocalKey
				if seenRuntimeKeys[key] {
					continue
				}
				seenRuntimeKeys[key] = true
			}
			decodeLocalCapabilities(capabilitiesJSON, &agent)
			applyAgentAccessForViewer(&agent, userID, role == domain.WorkspaceOwner || role == domain.WorkspaceAdmin, sharedSkillsJSON)
			agent.ResponseMode = domain.AgentResponseMention
			agent.MaxAutoTurns = 4
			agents = append(agents, agent)
		}
		return rows.Err()
	})
	return agents, err
}

func applyAgentAccessForViewer(agent *domain.AgentIdentity, userID domain.UserID, canModerate bool, sharedSkillsJSON string) {
	agent.CanManage = agent.OwnerUserID == userID
	agent.CanModerate = canModerate
	agent.CanInvoke = agent.CanManage || (agent.InvocationScope == domain.AgentInvocationRoomMembers && runnerSupportsScopedCapabilities(agent.RunnerVersion))
	_ = json.Unmarshal([]byte(sharedSkillsJSON), &agent.SharedSkills)
	if agent.SharedSkills == nil {
		agent.SharedSkills = []string{}
	}
	if agent.CanManage {
		return
	}
	allowed := make(map[string]bool, len(agent.SharedSkills))
	for _, name := range agent.SharedSkills {
		allowed[name] = true
	}
	filtered := make([]domain.AgentSkill, 0, len(agent.Skills))
	for _, skill := range agent.Skills {
		if allowed[skill.Name] {
			filtered = append(filtered, skill)
		}
	}
	agent.Skills = filtered
	agent.Resources = []domain.AgentResource{}
}

func runnerSupportsScopedCapabilities(version string) bool {
	version = strings.ToLower(strings.TrimSpace(version))
	// Local development builds intentionally report "dev". They use the same
	// scoped-task protocol as released Clients, so treating them as an unknown
	// legacy Client prevents every cross-account local collaboration flow.
	if version == "dev" || version == "development" {
		return true
	}
	version = strings.TrimPrefix(version, "v")
	if suffix := strings.IndexAny(version, "-+"); suffix >= 0 {
		version = version[:suffix]
	}
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil {
		return false
	}
	return major > 0 || minor >= 2
}

func decodeLocalCapabilities(raw string, agent *domain.AgentIdentity) {
	var capabilities struct {
		Transport       string                 `json:"transport"`
		CatalogTier     string                 `json:"catalog_tier"`
		ProtocolVersion int                    `json:"protocol_version"`
		Skills          []domain.AgentSkill    `json:"skills"`
		Resources       []domain.AgentResource `json:"resources"`
	}
	if json.Unmarshal([]byte(raw), &capabilities) != nil {
		agent.Transport = "native"
		agent.CatalogTier = "native"
		agent.Skills = []domain.AgentSkill{}
		agent.Resources = []domain.AgentResource{}
		return
	}
	agent.Transport = capabilities.Transport
	if agent.Transport == "" {
		agent.Transport = "native"
	}
	agent.CatalogTier = capabilities.CatalogTier
	if agent.CatalogTier == "" {
		agent.CatalogTier = "native"
	}
	agent.ProtocolVersion = capabilities.ProtocolVersion
	agent.Skills = capabilities.Skills
	agent.Resources = capabilities.Resources
	if agent.Skills == nil {
		agent.Skills = []domain.AgentSkill{}
	}
	if agent.Resources == nil {
		agent.Resources = []domain.AgentResource{}
	}
}

func (r *CollabRepository) ImportAgent(ctx context.Context, workspaceID domain.WorkspaceID, agentID domain.AgentID, userID domain.UserID) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := workspaceRoleTx(ctx, tx, workspaceID, userID); err != nil {
			return err
		}
		var sourceWorkspaceID domain.WorkspaceID
		var runnerID domain.RunnerID
		var ownerID domain.UserID
		var localKey, runtimeLocalKey, sourceDisplayName, rawDisplayName, agentType string
		var status domain.AgentStatus
		var capabilitiesJSON string
		if err := tx.QueryRowContext(ctx, `
SELECT workspace_id, runner_id, owner_user_id, local_key, COALESCE(NULLIF(runtime_local_key, ''), local_key),
       COALESCE(NULLIF(source_display_name, ''), display_name), display_name, type, status, capabilities_json
FROM agents
WHERE id = ? AND owner_user_id = ? AND status != 'unavailable'
  AND COALESCE(json_extract(capabilities_json, '$.execution'), 0) = 1
  AND EXISTS (SELECT 1 FROM agent_access_policies p WHERE p.agent_id = agents.id AND p.archived = 0)`, agentID, userID).Scan(
			&sourceWorkspaceID, &runnerID, &ownerID, &localKey, &runtimeLocalKey, &sourceDisplayName, &rawDisplayName, &agentType, &status, &capabilitiesJSON); err != nil {
			return mapNotFound(err)
		}
		if sourceWorkspaceID == workspaceID {
			result, err := tx.ExecContext(ctx, `UPDATE agents SET imported = 1, updated_at = ? WHERE id = ? AND workspace_id = ?`, formatTime(time.Now().UTC()), agentID, workspaceID)
			if err != nil {
				return err
			}
			if count, _ := result.RowsAffected(); count != 1 {
				return auth.ErrNotFound
			}
			return nil
		}

		var existingID domain.AgentID
		err := tx.QueryRowContext(ctx, `
SELECT id FROM agents
WHERE workspace_id = ? AND runner_id = ?
  AND COALESCE(NULLIF(runtime_local_key, ''), local_key) = ?`, workspaceID, runnerID, runtimeLocalKey).Scan(&existingID)
		if err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE agents SET imported = 1, updated_at = ? WHERE id = ?`, formatTime(time.Now().UTC()), existingID)
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if runtimeLocalKey == "" {
			runtimeLocalKey = localKey
		}
		if runtimeLocalKey == "" {
			return auth.ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO runner_agent_aliases (runner_id, legacy_agent_id, runtime_local_key)
VALUES (?, ?, ?)
ON CONFLICT(runner_id, legacy_agent_id) DO UPDATE SET runtime_local_key = excluded.runtime_local_key`, runnerID, agentID, runtimeLocalKey); err != nil {
			return err
		}
		cloneID, err := idgen.New("agent")
		if err != nil {
			return err
		}
		cloneLocalKey := runtimeLocalKey + "@" + string(workspaceID)
		cloneStoredName := sourceDisplayName + "@" + string(workspaceID)
		_, err = tx.ExecContext(ctx, `
INSERT INTO agents (id, workspace_id, runner_id, owner_user_id, local_key, runtime_local_key, source_display_name, type, display_name, status, capabilities_json, imported, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`, cloneID, workspaceID, runnerID, ownerID, cloneLocalKey, runtimeLocalKey, sourceDisplayName, agentType, cloneStoredName, status, capabilitiesJSON, formatTime(time.Now().UTC()), formatTime(time.Now().UTC()))
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO agent_access_policies (agent_id, updated_by_user_id) VALUES (?, ?)`, cloneID, userID)
		return err
	})
}

func (r *CollabRepository) RemoveAgent(ctx context.Context, workspaceID domain.WorkspaceID, agentID domain.AgentID, userID domain.UserID) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		role, err := workspaceRoleTx(ctx, tx, workspaceID, userID)
		if err != nil {
			return err
		}
		var ownerID domain.UserID
		if err := tx.QueryRowContext(ctx, `SELECT owner_user_id FROM agents WHERE id = ? AND workspace_id = ?`, agentID, workspaceID).Scan(&ownerID); err != nil {
			return mapNotFound(err)
		}
		if ownerID != userID && role != domain.WorkspaceOwner && role != domain.WorkspaceAdmin {
			return auth.ErrForbidden
		}
		var active int
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM tasks
WHERE workspace_id = ? AND target_agent_id = ? AND state IN ('queued', 'dispatched', 'running')`, workspaceID, agentID).Scan(&active); err != nil {
			return err
		}
		if active > 0 {
			return auth.ErrConflict
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM agent_sessions WHERE agent_id = ? AND room_id IN (SELECT id FROM rooms WHERE workspace_id = ?)`, agentID, workspaceID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM room_agent_settings WHERE agent_id = ? AND room_id IN (SELECT id FROM rooms WHERE workspace_id = ?)`, agentID, workspaceID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM room_agents WHERE agent_id = ? AND room_id IN (SELECT id FROM rooms WHERE workspace_id = ?)`, agentID, workspaceID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE agents SET imported = 0, updated_at = ? WHERE id = ? AND workspace_id = ?`, formatTime(time.Now().UTC()), agentID, workspaceID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return auth.ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `UPDATE agent_access_policies SET archived = 1, updated_by_user_id = ?, updated_at = ? WHERE agent_id = ?`, userID, formatTime(time.Now().UTC()), agentID); err != nil {
			return err
		}
		return nil
	})
}

func (r *CollabRepository) ListRoomAgents(ctx context.Context, roomID domain.RoomID, userID domain.UserID) ([]domain.AgentIdentity, error) {
	agents := make([]domain.AgentIdentity, 0)
	err := r.store.Read(ctx, func(db *sql.DB) error {
		var member int
		if err := db.QueryRowContext(ctx, `SELECT 1 FROM room_members WHERE room_id = ? AND user_id = ?`, roomID, userID).Scan(&member); err != nil {
			return mapNotFound(err)
		}
		rows, err := db.QueryContext(ctx, `
SELECT a.id, COALESCE(NULLIF(a.source_display_name, ''), a.display_name), a.type, a.owner_user_id, u.display_name, d.id, d.name, d.os, d.arch,
       'runner/' || lower(d.os), COALESCE(json_extract(a.capabilities_json, '$.permission_mode'), 'restricted'),
	       CASE
	         WHEN r.status != 'online' OR r.last_seen_at < ? THEN 'offline'
	         WHEN a.status = 'unavailable' THEN 'unavailable'
	         WHEN EXISTS (
	           SELECT 1 FROM tasks active JOIN agents task_agent ON task_agent.id = active.target_agent_id
	           WHERE task_agent.runner_id = a.runner_id
	             AND COALESCE(NULLIF(task_agent.runtime_local_key, ''), task_agent.local_key) = COALESCE(NULLIF(a.runtime_local_key, ''), a.local_key)
	             AND active.state IN ('dispatched', 'running')
	         ) THEN 'busy'
	         ELSE a.status
	       END,
       a.imported, COALESCE(json_extract(a.capabilities_json, '$.execution'), 0),
       COALESCE(json_extract(a.capabilities_json, '$.version'), ''), COALESCE(json_extract(a.capabilities_json, '$.executable'), ''),
	       COALESCE(json_extract(a.capabilities_json, '$.native_history'), ''), a.capabilities_json,
	       COALESCE(s.response_mode, 'mention'), COALESCE(s.reply_delay_seconds, 0),
		       COALESCE(s.role_prompt, ''), COALESCE(s.max_auto_turns, 4),
		       r.version, r.last_seen_at,
		       (SELECT COUNT(*) FROM tasks queued JOIN agents task_agent ON task_agent.id = queued.target_agent_id WHERE task_agent.runner_id = a.runner_id AND COALESCE(NULLIF(task_agent.runtime_local_key, ''), task_agent.local_key) = COALESCE(NULLIF(a.runtime_local_key, ''), a.local_key) AND queued.state = 'queued'),
		       (SELECT COUNT(*) FROM tasks active JOIN agents task_agent ON task_agent.id = active.target_agent_id WHERE task_agent.runner_id = a.runner_id AND COALESCE(NULLIF(task_agent.runtime_local_key, ''), task_agent.local_key) = COALESCE(NULLIF(a.runtime_local_key, ''), a.local_key) AND active.state IN ('dispatched', 'running')),
		       EXISTS (SELECT 1 FROM agent_sessions session WHERE session.agent_id = a.id AND session.state = 'active'),
		       p.visibility, p.invocation_scope, p.shared_skills_json, wm.role, viewer_rm.role
FROM room_agents ra
JOIN agents a ON a.id = ra.agent_id
JOIN runners r ON r.id = a.runner_id
JOIN devices d ON d.id = r.device_id
JOIN users u ON u.id = a.owner_user_id
JOIN agent_access_policies p ON p.agent_id = a.id
JOIN rooms room ON room.id = ra.room_id
JOIN workspace_members wm ON wm.workspace_id = room.workspace_id AND wm.user_id = ?
JOIN room_members viewer_rm ON viewer_rm.room_id = room.id AND viewer_rm.user_id = ?
LEFT JOIN room_agent_settings s ON s.room_id = ra.room_id AND s.agent_id = ra.agent_id
	WHERE ra.room_id = ? AND p.archived = 0 AND (p.visibility = 'workspace' OR a.owner_user_id = ?)
	ORDER BY ra.added_at, a.id`, formatTime(time.Now().UTC().Add(-runnerOnlineWindow)), userID, userID, roomID, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var agent domain.AgentIdentity
			var capabilitiesJSON, sharedSkillsJSON string
			var runnerLastSeenAt sql.NullString
			var workspaceRole domain.WorkspaceRole
			var roomRole domain.RoomRole
			if err := rows.Scan(&agent.AgentID, &agent.DisplayName, &agent.Type, &agent.OwnerUserID, &agent.OwnerName, &agent.DeviceID, &agent.DeviceName, &agent.OS, &agent.Arch, &agent.Channel, &agent.PermissionMode, &agent.Status, &agent.Imported, &agent.Available, &agent.Version, &agent.Executable, &agent.NativeHistory, &capabilitiesJSON, &agent.ResponseMode, &agent.ReplyDelaySeconds, &agent.RolePrompt, &agent.MaxAutoTurns, &agent.RunnerVersion, &runnerLastSeenAt, &agent.QueuedTaskCount, &agent.RunningTaskCount, &agent.NativeSessionReady, &agent.Visibility, &agent.InvocationScope, &sharedSkillsJSON, &workspaceRole, &roomRole); err != nil {
				return err
			}
			if runnerLastSeenAt.Valid {
				if parsed, parseErr := time.Parse(time.RFC3339Nano, runnerLastSeenAt.String); parseErr == nil {
					agent.RunnerLastSeenAt = &parsed
				}
			}
			decodeLocalCapabilities(capabilitiesJSON, &agent)
			canModerate := workspaceRole == domain.WorkspaceOwner || workspaceRole == domain.WorkspaceAdmin || roomRole == domain.RoomOwner || roomRole == domain.RoomAdmin
			applyAgentAccessForViewer(&agent, userID, canModerate, sharedSkillsJSON)
			agents = append(agents, agent)
		}
		return rows.Err()
	})
	return agents, err
}

func (r *CollabRepository) AddRoomAgent(ctx context.Context, roomID domain.RoomID, agentID domain.AgentID, userID domain.UserID) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		var roomWorkspaceID, agentWorkspaceID domain.WorkspaceID
		var agentOwnerID domain.UserID
		var visibility domain.AgentVisibility
		if err := tx.QueryRowContext(ctx, `
SELECT rooms.workspace_id FROM rooms
JOIN room_members ON room_members.room_id = rooms.id
WHERE rooms.id = ? AND room_members.user_id = ?`, roomID, userID).Scan(&roomWorkspaceID); err != nil {
			return mapNotFound(err)
		}
		if err := tx.QueryRowContext(ctx, `SELECT a.workspace_id, a.owner_user_id, p.visibility FROM agents a JOIN agent_access_policies p ON p.agent_id = a.id WHERE a.id = ? AND a.imported = 1 AND a.status != 'unavailable' AND p.archived = 0`, agentID).Scan(&agentWorkspaceID, &agentOwnerID, &visibility); err != nil {
			return mapNotFound(err)
		}
		if roomWorkspaceID != agentWorkspaceID {
			return auth.ErrForbidden
		}
		role, err := workspaceRoleTx(ctx, tx, roomWorkspaceID, userID)
		if err != nil {
			return err
		}
		roomRole, err := roomRoleTx(ctx, tx, roomID, userID)
		if err != nil {
			return err
		}
		if agentOwnerID != userID && role != domain.WorkspaceOwner && role != domain.WorkspaceAdmin && roomRole != domain.RoomOwner && roomRole != domain.RoomAdmin {
			return auth.ErrForbidden
		}
		if agentOwnerID != userID && visibility != domain.AgentVisibilityWorkspace {
			return auth.ErrForbidden
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO room_agents (room_id, agent_id, added_by_user_id)
VALUES (?, ?, ?)
ON CONFLICT(room_id, agent_id) DO NOTHING`, roomID, agentID, userID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO room_agent_settings (room_id, agent_id, updated_by_user_id)
VALUES (?, ?, ?)
ON CONFLICT(room_id, agent_id) DO NOTHING`, roomID, agentID, userID)
		return err
	})
}

func (r *CollabRepository) RemoveRoomAgent(ctx context.Context, roomID domain.RoomID, agentID domain.AgentID, userID domain.UserID) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		var workspaceID domain.WorkspaceID
		var agentOwnerID domain.UserID
		if err := tx.QueryRowContext(ctx, `
SELECT r.workspace_id FROM rooms r
JOIN room_members rm ON rm.room_id = r.id
JOIN room_agents ra ON ra.room_id = r.id
WHERE r.id = ? AND rm.user_id = ? AND ra.agent_id = ?`, roomID, userID, agentID).Scan(&workspaceID); err != nil {
			return mapNotFound(err)
		}
		if err := tx.QueryRowContext(ctx, `SELECT owner_user_id FROM agents WHERE id = ?`, agentID).Scan(&agentOwnerID); err != nil {
			return mapNotFound(err)
		}
		role, err := workspaceRoleTx(ctx, tx, workspaceID, userID)
		if err != nil {
			return err
		}
		roomRole, err := roomRoleTx(ctx, tx, roomID, userID)
		if err != nil {
			return err
		}
		if role != domain.WorkspaceOwner && role != domain.WorkspaceAdmin && roomRole != domain.RoomOwner && roomRole != domain.RoomAdmin && agentOwnerID != userID {
			return auth.ErrForbidden
		}
		var active int
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM tasks
WHERE room_id = ? AND target_agent_id = ? AND state IN ('queued', 'dispatched', 'running')`, roomID, agentID).Scan(&active); err != nil {
			return err
		}
		if active > 0 {
			return auth.ErrConflict
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM agent_sessions WHERE room_id = ? AND agent_id = ?`, roomID, agentID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM room_agent_settings WHERE room_id = ? AND agent_id = ?`, roomID, agentID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM room_agents WHERE room_id = ? AND agent_id = ?`, roomID, agentID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return auth.ErrNotFound
		}
		return nil
	})
}

func (r *CollabRepository) UpdateRoomAgentSettings(ctx context.Context, settings domain.RoomAgentSettings, userID domain.UserID) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		var member int
		if err := tx.QueryRowContext(ctx, `
SELECT 1 FROM room_members rm JOIN room_agents ra ON ra.room_id = rm.room_id
WHERE rm.room_id = ? AND rm.user_id = ? AND ra.agent_id = ?`, settings.RoomID, userID, settings.AgentID).Scan(&member); err != nil {
			return mapNotFound(err)
		}
		var ownerID domain.UserID
		if err := tx.QueryRowContext(ctx, `SELECT a.owner_user_id FROM rooms r JOIN agents a ON a.id = ? WHERE r.id = ?`, settings.AgentID, settings.RoomID).Scan(&ownerID); err != nil {
			return mapNotFound(err)
		}
		if ownerID != userID {
			return auth.ErrForbidden
		}
		if settings.ResponseMode == domain.AgentResponseCoordinator {
			var foreignCoordinator int
			if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM room_agent_settings s
JOIN agents a ON a.id = s.agent_id
WHERE s.room_id = ? AND s.agent_id != ? AND s.response_mode = 'coordinator' AND a.owner_user_id != ?`, settings.RoomID, settings.AgentID, userID).Scan(&foreignCoordinator); err != nil {
				return err
			}
			if foreignCoordinator > 0 {
				return auth.ErrConflict
			}
			if _, err := tx.ExecContext(ctx, `
UPDATE room_agent_settings SET response_mode = 'mention', updated_by_user_id = ?, updated_at = ?
WHERE room_id = ? AND agent_id != ? AND response_mode = 'coordinator'
  AND agent_id IN (SELECT id FROM agents WHERE owner_user_id = ?)`, userID, formatTime(time.Now().UTC()), settings.RoomID, settings.AgentID, userID); err != nil {
				return err
			}
		}
		result, err := tx.ExecContext(ctx, `
UPDATE room_agent_settings
SET response_mode = ?, reply_delay_seconds = ?, role_prompt = ?, max_auto_turns = ?, updated_by_user_id = ?, updated_at = ?
WHERE room_id = ? AND agent_id = ?`, settings.ResponseMode, settings.ReplyDelaySeconds, settings.RolePrompt,
			settings.MaxAutoTurns, userID, formatTime(time.Now().UTC()), settings.RoomID, settings.AgentID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return auth.ErrNotFound
		}
		return nil
	})
}

func (r *CollabRepository) CreateTask(ctx context.Context, task domain.Task, userID domain.UserID) (domain.Task, domain.Message, error) {
	var message domain.Message
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
SELECT rooms.workspace_id FROM rooms
JOIN room_members ON room_members.room_id = rooms.id
WHERE rooms.id = ? AND room_members.user_id = ?`, task.RoomID, userID).Scan(&task.WorkspaceID); err != nil {
			return mapNotFound(err)
		}
		var runnerID domain.RunnerID
		var ownerID domain.UserID
		var invocation domain.AgentInvocationScope
		var runnerVersion string
		if err := tx.QueryRowContext(ctx, `
	SELECT a.runner_id, a.owner_user_id, p.invocation_scope, r.version FROM agents a
	JOIN runners r ON r.id = a.runner_id
	JOIN room_agents ra ON ra.agent_id = a.id
	JOIN agent_access_policies p ON p.agent_id = a.id
	WHERE a.id = ? AND a.workspace_id = ? AND ra.room_id = ? AND a.status != 'unavailable' AND p.archived = 0`, task.TargetAgentID, task.WorkspaceID, task.RoomID).Scan(&runnerID, &ownerID, &invocation, &runnerVersion); err != nil {
			return mapNotFound(err)
		}
		if ownerID != userID && (invocation != domain.AgentInvocationRoomMembers || !runnerSupportsScopedCapabilities(runnerVersion)) {
			return auth.ErrForbidden
		}
		if !task.ExecutionSource.Valid() {
			return auth.ErrInvalidInput
		}
		attemptID, err := idgen.New("attempt")
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO tasks (id, workspace_id, room_id, parent_task_id, creator_kind, creator_id, target_agent_id, title, prompt, acceptance_criteria,
                   execution_source, state, current_attempt, root_message_id, source_message_id, collaboration_depth, created_at, updated_at)
VALUES (?, ?, ?, ?, 'user', ?, ?, ?, ?, ?, ?, 'queued', 1, ?, ?, ?, ?, ?)`, task.ID, task.WorkspaceID, task.RoomID, task.ParentTaskID,
			userID, task.TargetAgentID, task.Title, task.Prompt, task.AcceptanceCriteria, task.ExecutionSource, task.RootMessageID, task.SourceMessageID,
			task.CollaborationDepth, formatTime(task.CreatedAt), formatTime(task.UpdatedAt)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO task_attempts (id, task_id, attempt_no, state, created_at) VALUES (?, ?, 1, 'queued', ?)`, attemptID, task.ID, formatTime(task.CreatedAt)); err != nil {
			return err
		}
		messageID, err := idgen.New("msg")
		if err != nil {
			return err
		}
		sequence, err := allocateSequence(ctx, tx, task.WorkspaceID)
		if err != nil {
			return err
		}
		metadataValues := map[string]any{"task_id": task.ID, "task_title": task.Title, "task_source": "structured"}
		if task.PlanStepID != nil {
			metadataValues["plan_step_id"] = *task.PlanStepID
		}
		metadata, _ := json.Marshal(metadataValues)
		message = domain.Message{ID: domain.MessageID(messageID), WorkspaceID: task.WorkspaceID, RoomID: task.RoomID, Sequence: sequence, SenderKind: domain.MessageSenderUser, SenderID: string(userID), Kind: domain.MessageChat, Body: task.Prompt, MentionAgentIDs: []domain.AgentID{task.TargetAgentID}, Metadata: metadata, CreatedAt: task.CreatedAt}
		if _, err = tx.ExecContext(ctx, `
INSERT INTO messages (message_id, workspace_id, room_id, sequence, sender_kind, sender_id, kind, body, mention_agent_ids_json, metadata_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, message.ID, message.WorkspaceID, message.RoomID, message.Sequence, message.SenderKind, message.SenderID, message.Kind, message.Body, `[`+strconv.Quote(string(task.TargetAgentID))+`]`, string(metadata), formatTime(message.CreatedAt)); err != nil {
			return err
		}
		if task.RootMessageID == nil {
			root := message.ID
			task.RootMessageID = &root
		}
		if task.SourceMessageID == nil {
			source := message.ID
			task.SourceMessageID = &source
		}
		workItemID, err := ensureWorkItemForRootTx(ctx, tx, task.WorkspaceID, task.RoomID, *task.RootMessageID, task.Title, task.CreatedAt)
		if err != nil {
			return err
		}
		task.WorkItemID = &workItemID
		if err := validateTaskExecutionSourceTx(ctx, tx, task.ExecutionSource, workItemID, task.TargetAgentID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE tasks SET root_message_id = ?, source_message_id = ?, work_item_id = ? WHERE id = ?`, *task.RootMessageID, *task.SourceMessageID, workItemID, task.ID)
		return err
	})
	return task, message, err
}

func (r *CollabRepository) UpdateAgentAccessPolicy(ctx context.Context, workspaceID domain.WorkspaceID, policy domain.AgentAccessPolicy, userID domain.UserID) error {
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := workspaceRoleTx(ctx, tx, workspaceID, userID); err != nil {
			return err
		}
		var capabilitiesJSON string
		if err := tx.QueryRowContext(ctx, `SELECT capabilities_json FROM agents WHERE id = ? AND workspace_id = ? AND owner_user_id = ?`, policy.AgentID, workspaceID, userID).Scan(&capabilitiesJSON); err != nil {
			return mapNotFound(err)
		}
		var capabilities struct {
			Skills []domain.AgentSkill `json:"skills"`
		}
		_ = json.Unmarshal([]byte(capabilitiesJSON), &capabilities)
		available := make(map[string]bool, len(capabilities.Skills))
		for _, skill := range capabilities.Skills {
			available[skill.Name] = true
		}
		seen := make(map[string]bool)
		shared := make([]string, 0, len(policy.SharedSkills))
		for _, name := range policy.SharedSkills {
			name = strings.TrimSpace(name)
			if name == "" || !available[name] || seen[name] {
				continue
			}
			seen[name] = true
			shared = append(shared, name)
		}
		sharedJSON, _ := json.Marshal(shared)
		result, err := tx.ExecContext(ctx, `UPDATE agent_access_policies SET visibility = ?, invocation_scope = ?, shared_skills_json = ?, updated_by_user_id = ?, updated_at = ? WHERE agent_id = ? AND archived = 0`, policy.Visibility, policy.InvocationScope, string(sharedJSON), userID, formatTime(time.Now().UTC()), policy.AgentID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return auth.ErrNotFound
		}
		if policy.Visibility == domain.AgentVisibilityPrivate {
			if _, err := tx.ExecContext(ctx, `DELETE FROM room_agent_settings WHERE agent_id = ? AND room_id IN (SELECT room_id FROM room_agents WHERE agent_id = ? AND room_id IN (SELECT rm.room_id FROM room_members rm WHERE rm.user_id != ?))`, policy.AgentID, policy.AgentID, userID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM room_agents WHERE agent_id = ? AND room_id IN (SELECT rm.room_id FROM room_members rm WHERE rm.user_id != ?)`, policy.AgentID, userID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *CollabRepository) ListRoomTasks(ctx context.Context, roomID domain.RoomID, userID domain.UserID) ([]domain.Task, error) {
	tasks := make([]domain.Task, 0)
	err := r.store.Read(ctx, func(db *sql.DB) error {
		var member int
		if err := db.QueryRowContext(ctx, `SELECT 1 FROM room_members WHERE room_id = ? AND user_id = ?`, roomID, userID).Scan(&member); err != nil {
			return mapNotFound(err)
		}
		rows, err := db.QueryContext(ctx, `
SELECT id, work_item_id, workspace_id, room_id, parent_task_id, root_message_id, source_message_id, collaboration_depth,
	       creator_kind, creator_id, target_agent_id, title, prompt, acceptance_criteria, execution_source, state, current_attempt, created_at, updated_at
FROM tasks
WHERE room_id = ?
ORDER BY created_at DESC, id DESC
LIMIT 100`, roomID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var task domain.Task
			var workItemID, parentTaskID, rootMessageID, sourceMessageID sql.NullString
			var createdAt, updatedAt string
			if err := rows.Scan(&task.ID, &workItemID, &task.WorkspaceID, &task.RoomID, &parentTaskID, &rootMessageID, &sourceMessageID,
				&task.CollaborationDepth, &task.CreatorKind, &task.CreatorID, &task.TargetAgentID, &task.Title, &task.Prompt,
				&task.AcceptanceCriteria, &task.ExecutionSource, &task.State, &task.CurrentAttempt, &createdAt, &updatedAt); err != nil {
				return err
			}
			if workItemID.Valid {
				value := domain.WorkItemID(workItemID.String)
				task.WorkItemID = &value
			}
			if parentTaskID.Valid {
				parent := domain.TaskID(parentTaskID.String)
				task.ParentTaskID = &parent
			}
			if rootMessageID.Valid {
				root := domain.MessageID(rootMessageID.String)
				task.RootMessageID = &root
			}
			if sourceMessageID.Valid {
				source := domain.MessageID(sourceMessageID.String)
				task.SourceMessageID = &source
			}
			task.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
			task.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
			tasks = append(tasks, task)
		}
		return rows.Err()
	})
	for left, right := 0, len(tasks)-1; left < right; left, right = left+1, right-1 {
		tasks[left], tasks[right] = tasks[right], tasks[left]
	}
	return tasks, err
}

// CancelTasksForRoot stops every unfinished task that belongs to one visible
// human topic. The root author can stop their own delegation; room owners and
// administrators can stop any topic. Completed deliveries remain untouched so
// the conversation and its audit trail never lose already returned work.
func (r *CollabRepository) CancelTasksForRoot(ctx context.Context, roomID domain.RoomID, rootMessageID domain.MessageID, userID domain.UserID) ([]domain.Task, domain.Message, error) {
	tasks := make([]domain.Task, 0)
	var message domain.Message
	now := time.Now().UTC()
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		role, err := roomRoleTx(ctx, tx, roomID, userID)
		if err != nil {
			return err
		}
		var rootAuthorID domain.UserID
		var senderKind domain.SenderKind
		if err := tx.QueryRowContext(ctx, `
SELECT sender_id, sender_kind FROM messages
WHERE message_id = ? AND room_id = ?`, rootMessageID, roomID).Scan(&rootAuthorID, &senderKind); err != nil {
			return mapNotFound(err)
		}
		if senderKind != domain.MessageSenderUser {
			return auth.ErrInvalidInput
		}
		if rootAuthorID != userID && role != domain.RoomOwner && role != domain.RoomAdmin {
			return auth.ErrForbidden
		}

		rows, err := tx.QueryContext(ctx, `
SELECT id, work_item_id, workspace_id, room_id, parent_task_id, root_message_id, source_message_id, collaboration_depth,
	       creator_kind, creator_id, target_agent_id, title, prompt, acceptance_criteria, execution_source, state, current_attempt, created_at, updated_at
FROM tasks
WHERE room_id = ? AND root_message_id = ? AND state IN ('queued', 'dispatched', 'running')
ORDER BY created_at, id`, roomID, rootMessageID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var task domain.Task
			var workItemID, parentTaskID, storedRootID, sourceMessageID sql.NullString
			var createdAt, updatedAt string
			if err := rows.Scan(&task.ID, &workItemID, &task.WorkspaceID, &task.RoomID, &parentTaskID, &storedRootID, &sourceMessageID,
				&task.CollaborationDepth, &task.CreatorKind, &task.CreatorID, &task.TargetAgentID, &task.Title, &task.Prompt,
				&task.AcceptanceCriteria, &task.ExecutionSource, &task.State, &task.CurrentAttempt, &createdAt, &updatedAt); err != nil {
				return err
			}
			if workItemID.Valid {
				value := domain.WorkItemID(workItemID.String)
				task.WorkItemID = &value
			}
			if parentTaskID.Valid {
				value := domain.TaskID(parentTaskID.String)
				task.ParentTaskID = &value
			}
			if storedRootID.Valid {
				value := domain.MessageID(storedRootID.String)
				task.RootMessageID = &value
			}
			if sourceMessageID.Valid {
				value := domain.MessageID(sourceMessageID.String)
				task.SourceMessageID = &value
			}
			task.State = domain.TaskCancelled
			task.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
			task.UpdatedAt = now
			tasks = append(tasks, task)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(tasks) == 0 {
			return nil
		}

		if _, err := tx.ExecContext(ctx, `
UPDATE tasks SET state = 'cancelled', updated_at = ?
WHERE room_id = ? AND root_message_id = ? AND state IN ('queued', 'dispatched', 'running')`, formatTime(now), roomID, rootMessageID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE task_attempts
SET state = 'cancelled', finished_at = ?, error_summary = '用户已停止本次委托'
WHERE task_id IN (
    SELECT id FROM tasks WHERE room_id = ? AND root_message_id = ? AND state = 'cancelled'
) AND state IN ('queued', 'dispatched', 'running')`, formatTime(now), roomID, rootMessageID); err != nil {
			return err
		}
		if tasks[0].WorkItemID != nil {
			if err := syncWorkItemStateTx(ctx, tx, *tasks[0].WorkItemID, now); err != nil {
				return err
			}
		}

		messageID, err := idgen.New("msg")
		if err != nil {
			return err
		}
		workspaceID := tasks[0].WorkspaceID
		sequence, err := allocateSequence(ctx, tx, workspaceID)
		if err != nil {
			return err
		}
		cancelledIDs := make([]string, 0, len(tasks))
		for _, task := range tasks {
			cancelledIDs = append(cancelledIDs, string(task.ID))
		}
		metadata, _ := json.Marshal(map[string]any{"task_state": domain.TaskCancelled, "cancelled_task_ids": cancelledIDs})
		message = domain.Message{
			ID:               domain.MessageID(messageID),
			WorkspaceID:      workspaceID,
			RoomID:           roomID,
			Sequence:         sequence,
			SenderKind:       domain.MessageSenderSystem,
			SenderID:         "system",
			Kind:             domain.MessageProgress,
			Body:             fmt.Sprintf("已停止本次委托：已取消 %d 项尚未完成的 Agent 执行。已产生的回复会保留在话题中。", len(tasks)),
			ReplyToMessageID: &rootMessageID,
			RootMessageID:    &rootMessageID,
			Metadata:         metadata,
			CreatedAt:        now,
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO messages (message_id, workspace_id, room_id, sequence, sender_kind, sender_id, kind, body,
                      mention_agent_ids_json, reply_to_message_id, root_message_id, metadata_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, '[]', ?, ?, ?, ?)`, message.ID, message.WorkspaceID, message.RoomID, message.Sequence,
			message.SenderKind, message.SenderID, message.Kind, message.Body, message.ReplyToMessageID, message.RootMessageID,
			string(metadata), formatTime(now))
		return err
	})
	return tasks, message, err
}

// TaskCancellationRequested is intentionally runner-scoped. A Runner can
// only observe cancellation for a task it has already claimed for one of its
// local Agents; every other task is hidden as not found.
func (r *CollabRepository) TaskCancellationRequested(ctx context.Context, runnerID domain.RunnerID, taskID domain.TaskID) (bool, error) {
	var state domain.TaskState
	err := r.store.Read(ctx, func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `
SELECT t.state FROM tasks t
JOIN agents a ON a.id = t.target_agent_id
WHERE t.id = ? AND a.runner_id = ?`, taskID, runnerID).Scan(&state)
	})
	if err != nil {
		return false, mapNotFound(err)
	}
	return state == domain.TaskCancelled, nil
}

// RecoverInterruptedTasks releases work claimed by an older process with the
// same Runner identity. The Runner invokes it only while starting; its child
// CLI processes were terminated with that prior process, so retrying the
// unfinished task is safer than keeping a permanent running state.
func (r *CollabRepository) RecoverInterruptedTasks(ctx context.Context, runnerID domain.RunnerID) (int, error) {
	now := time.Now().UTC()
	requeued := 0
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
UPDATE tasks
SET state = 'queued', updated_at = ?
WHERE state IN ('dispatched', 'running')
  AND EXISTS (
      SELECT 1 FROM task_attempts active
      WHERE active.task_id = tasks.id
        AND active.attempt_no = tasks.current_attempt
        AND active.runner_id = ?
        AND active.state IN ('dispatched', 'running')
  )`, formatTime(now), runnerID)
		if err != nil {
			return err
		}
		if count, err := result.RowsAffected(); err != nil {
			return err
		} else {
			requeued = int(count)
		}
		_, err = tx.ExecContext(ctx, `
UPDATE task_attempts
SET state = 'queued', runner_id = NULL, started_at = NULL, finished_at = NULL, exit_code = NULL, error_summary = NULL
WHERE runner_id = ?
  AND state IN ('dispatched', 'running')
  AND attempt_no = (SELECT current_attempt FROM tasks WHERE tasks.id = task_attempts.task_id)
  AND task_id IN (SELECT id FROM tasks WHERE state = 'queued')`, runnerID)
		return err
	})
	return requeued, err
}

func (r *CollabRepository) ClaimNextTask(ctx context.Context, runnerID domain.RunnerID) (*domain.ClaimedTask, error) {
	var claimed *domain.ClaimedTask
	now := time.Now().UTC()
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		var task domain.ClaimedTask
		var createdAt, updatedAt string
		var parentTaskID, rootMessageID, sourceMessageID, nativeSessionID sql.NullString
		var planStepID, workspaceSourceName, workspaceCommit, localDirectoryLabel, localBaselineCommit sql.NullString
		var contextObjective, contextStepTitle, contextStepDescription, contextStepStatus sql.NullString
		var workspaceRevision, workspaceFileCount, workspaceTotalBytes sql.NullInt64
		var targetOwnerID, effectiveUserID domain.UserID
		var sharedSkillsJSON string
		err := tx.QueryRowContext(ctx, `
SELECT t.id, t.workspace_id, t.room_id, t.parent_task_id, t.root_message_id, t.source_message_id, t.collaboration_depth,
	       t.creator_kind, t.creator_id, t.target_agent_id,
	       COALESCE((
	           SELECT source.id FROM agents source
	           WHERE source.runner_id = a.runner_id
	             AND source.runtime_local_key = COALESCE(NULLIF(a.runtime_local_key, ''), a.local_key)
	             AND source.local_key NOT LIKE '%@%'
	           ORDER BY source.created_at, source.id LIMIT 1
	       ), a.id),
	       COALESCE(NULLIF(a.runtime_local_key, ''), a.local_key), a.type, t.title, t.prompt, t.acceptance_criteria, t.execution_source, t.state, t.current_attempt,
	       t.created_at, t.updated_at, s.native_session_id, a.owner_user_id, p.shared_skills_json,
	       CASE WHEN t.creator_kind = 'user' THEN t.creator_id ELSE COALESCE((SELECT sender_id FROM messages m WHERE m.message_id = t.root_message_id AND m.sender_kind = 'user'), '') END,
	       wi.plan_step_id,
	       CASE WHEN t.project_workspace_commit <> '' THEN t.project_workspace_source_name ELSE psw.source_name END,
	       CASE WHEN t.project_workspace_commit <> '' THEN t.project_workspace_revision ELSE psw.revision END,
	       COALESCE(NULLIF(t.project_workspace_commit, ''), psw.last_commit),
	       CASE WHEN t.project_workspace_commit <> '' THEN t.project_workspace_file_count ELSE psw.file_count END,
	       CASE WHEN t.project_workspace_commit <> '' THEN t.project_workspace_total_bytes ELSE psw.total_bytes END,
	       lb.directory_label, lb.baseline_commit,
	       pp.objective, ps.title, ps.description, ps.status
FROM tasks t JOIN agents a ON a.id = t.target_agent_id
JOIN agent_access_policies p ON p.agent_id = a.id
LEFT JOIN agent_sessions s ON s.room_id = t.room_id AND s.agent_id = t.target_agent_id AND s.state = 'active'
	LEFT JOIN work_items wi ON wi.id = t.work_item_id
	LEFT JOIN project_steps ps ON ps.id = wi.plan_step_id
	LEFT JOIN project_plans pp ON pp.workspace_id = t.workspace_id
	LEFT JOIN project_step_workspaces psw ON psw.step_id = wi.plan_step_id
	LEFT JOIN project_step_local_bindings lb ON lb.step_id = wi.plan_step_id AND lb.user_id = a.owner_user_id AND lb.runner_id = a.runner_id
WHERE a.runner_id = ? AND t.state = 'queued'
  AND NOT EXISTS (
      SELECT 1 FROM tasks active
      WHERE active.target_agent_id = t.target_agent_id
        AND active.state IN ('dispatched', 'running')
  )
		ORDER BY t.created_at, t.id LIMIT 1`, runnerID).Scan(&task.ID, &task.WorkspaceID, &task.RoomID, &parentTaskID,
			&rootMessageID, &sourceMessageID, &task.CollaborationDepth, &task.CreatorKind, &task.CreatorID, &task.TargetAgentID,
			&task.TargetAgentSourceID, &task.TargetAgentLocalKey, &task.TargetAgentType, &task.Title, &task.Prompt, &task.AcceptanceCriteria, &task.ExecutionSource, &task.State, &task.CurrentAttempt, &createdAt, &updatedAt, &nativeSessionID, &targetOwnerID, &sharedSkillsJSON, &effectiveUserID,
			&planStepID, &workspaceSourceName, &workspaceRevision, &workspaceCommit, &workspaceFileCount, &workspaceTotalBytes, &localDirectoryLabel, &localBaselineCommit,
			&contextObjective, &contextStepTitle, &contextStepDescription, &contextStepStatus)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		task.AttemptNo = task.CurrentAttempt
		if targetOwnerID == effectiveUserID {
			task.CapabilityScope = "owner_all"
		} else {
			task.CapabilityScope = "shared_only"
			_ = json.Unmarshal([]byte(sharedSkillsJSON), &task.AuthorizedSkills)
			if task.AuthorizedSkills == nil {
				task.AuthorizedSkills = []string{}
			}
		}
		if parentTaskID.Valid {
			value := domain.TaskID(parentTaskID.String)
			task.ParentTaskID = &value
		}
		if rootMessageID.Valid {
			value := domain.MessageID(rootMessageID.String)
			task.RootMessageID = &value
		}
		if sourceMessageID.Valid {
			value := domain.MessageID(sourceMessageID.String)
			task.SourceMessageID = &value
		}
		task.Envelope = task.Task.Envelope()
		if nativeSessionID.Valid {
			task.NativeSessionID = nativeSessionID.String
		}
		if planStepID.Valid {
			value := domain.ProjectStepID(planStepID.String)
			task.PlanStepID = &value
			task.ProjectContext = &domain.ProjectContextSnapshot{
				Objective: contextObjective.String, StepID: value, StepTitle: contextStepTitle.String,
				StepDescription: contextStepDescription.String, StepStatus: domain.ProjectStepStatus(contextStepStatus.String),
			}
		}
		if task.ExecutionSource == domain.TaskExecutionResponsibleLocal && planStepID.Valid && localDirectoryLabel.Valid {
			task.LocalWorkspace = &domain.LocalProjectWorkspace{StepID: domain.ProjectStepID(planStepID.String), DirectoryLabel: localDirectoryLabel.String, BaselineCommit: localBaselineCommit.String}
		}
		if task.ExecutionSource != domain.TaskExecutionResponsibleLocal && task.ExecutionSource != domain.TaskExecutionNone && planStepID.Valid && workspaceCommit.Valid && strings.TrimSpace(workspaceCommit.String) != "" {
			task.ProjectWorkspace = &domain.ProjectWorkspaceSnapshot{
				WorkspaceID: task.WorkspaceID, StepID: domain.ProjectStepID(planStepID.String), SourceName: workspaceSourceName.String,
				Revision: int(workspaceRevision.Int64), Commit: strings.TrimSpace(workspaceCommit.String),
				FileCount: int(workspaceFileCount.Int64), TotalBytes: workspaceTotalBytes.Int64,
				DownloadURL: "/api/v1/runner/tasks/" + string(task.ID) + "/project-workspace",
			}
		}
		if task.SourceMessageID != nil {
			attachments, err := listAttachmentsTx(ctx, tx, *task.SourceMessageID)
			if err != nil {
				return err
			}
			task.Attachments = attachments
		}
		knowledgeAssets, err := listKnowledgeAssetsForTaskTx(ctx, tx, task.WorkspaceID, task.RoomID)
		if err != nil {
			return err
		}
		task.KnowledgeAssets = knowledgeAssets
		task.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		task.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		frozenCommit := ""
		if task.ProjectWorkspace != nil {
			frozenCommit = task.ProjectWorkspace.Commit
		}
		frozenSourceName := ""
		frozenRevision, frozenFileCount := 0, 0
		var frozenTotalBytes int64
		if task.ProjectWorkspace != nil {
			frozenSourceName = task.ProjectWorkspace.SourceName
			frozenRevision = task.ProjectWorkspace.Revision
			frozenFileCount = task.ProjectWorkspace.FileCount
			frozenTotalBytes = task.ProjectWorkspace.TotalBytes
		}
		if _, err := tx.ExecContext(ctx, `UPDATE tasks SET state = 'dispatched',
			project_workspace_commit = CASE WHEN project_workspace_commit = '' THEN ? ELSE project_workspace_commit END,
			project_workspace_source_name = CASE WHEN project_workspace_commit = '' THEN ? ELSE project_workspace_source_name END,
			project_workspace_revision = CASE WHEN project_workspace_commit = '' THEN ? ELSE project_workspace_revision END,
			project_workspace_file_count = CASE WHEN project_workspace_commit = '' THEN ? ELSE project_workspace_file_count END,
			project_workspace_total_bytes = CASE WHEN project_workspace_commit = '' THEN ? ELSE project_workspace_total_bytes END,
			updated_at = ? WHERE id = ? AND state = 'queued'`, frozenCommit, frozenSourceName, frozenRevision, frozenFileCount, frozenTotalBytes, formatTime(now), task.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE task_attempts SET state = 'dispatched', runner_id = ? WHERE task_id = ? AND attempt_no = ? AND state = 'queued'`, runnerID, task.ID, task.AttemptNo); err != nil {
			return err
		}
		task.State = domain.TaskDispatched
		task.UpdatedAt = now
		claimed = &task
		return nil
	})
	return claimed, err
}

// ProjectWorkspaceForRunnerTask exposes only the immutable step snapshot that
// was frozen when this Runner claimed the task.
func (r *CollabRepository) ProjectWorkspaceForRunnerTask(ctx context.Context, taskID domain.TaskID, runnerID domain.RunnerID) (domain.ProjectWorkspaceSnapshot, error) {
	var snapshot domain.ProjectWorkspaceSnapshot
	err := r.store.Read(ctx, func(db *sql.DB) error {
		return db.QueryRowContext(ctx, `
SELECT t.workspace_id, wi.plan_step_id, t.project_workspace_source_name, t.project_workspace_revision,
       t.project_workspace_commit, t.project_workspace_file_count, t.project_workspace_total_bytes
FROM tasks t
JOIN agents a ON a.id = t.target_agent_id
JOIN work_items wi ON wi.id = t.work_item_id
JOIN project_step_workspaces psw ON psw.step_id = wi.plan_step_id
WHERE t.id = ? AND a.runner_id = ? AND t.project_workspace_commit <> ''
  AND t.state IN ('dispatched', 'running', 'delivered', 'failed')`, taskID, runnerID).Scan(
			&snapshot.WorkspaceID, &snapshot.StepID, &snapshot.SourceName, &snapshot.Revision, &snapshot.Commit, &snapshot.FileCount, &snapshot.TotalBytes,
		)
	})
	if err != nil {
		return domain.ProjectWorkspaceSnapshot{}, mapNotFound(err)
	}
	snapshot.DownloadURL = "/api/v1/runner/tasks/" + string(taskID) + "/project-workspace"
	return snapshot, nil
}

func (r *CollabRepository) StartTask(ctx context.Context, runnerID domain.RunnerID, taskID domain.TaskID) error {
	now := time.Now().UTC()
	return r.store.Write(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
UPDATE tasks SET state = 'running', updated_at = ?
WHERE id = ? AND state = 'dispatched' AND target_agent_id IN (SELECT id FROM agents WHERE runner_id = ?)`, formatTime(now), taskID, runnerID)
		if err != nil {
			return err
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return auth.ErrConflict
		}
		_, err = tx.ExecContext(ctx, `UPDATE task_attempts SET state = 'running', started_at = ? WHERE task_id = ? AND attempt_no = (SELECT current_attempt FROM tasks WHERE id = ?) AND runner_id = ?`, formatTime(now), taskID, taskID, runnerID)
		return err
	})
}

func directCollaborationBatchReadyTx(ctx context.Context, tx *sql.Tx, parentTaskID domain.TaskID) (bool, error) {
	var pending int
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM tasks
WHERE parent_task_id = ?
  AND title NOT LIKE ?
  AND state IN ('queued', 'dispatched', 'running')`, parentTaskID, agentCallbackTitlePrefix+"%").Scan(&pending); err != nil {
		return false, err
	}
	return pending == 0, nil
}

func collaborationCallbackExistsTx(ctx context.Context, tx *sql.Tx, parentTaskID domain.TaskID, targetAgentID domain.AgentID) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM tasks
WHERE parent_task_id = ? AND target_agent_id = ? AND title LIKE ?`, parentTaskID, targetAgentID, agentCallbackTitlePrefix+"%").Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func collaborationBatchSummaryTx(ctx context.Context, tx *sql.Tx, parentTaskID domain.TaskID) (string, error) {
	type result struct {
		taskID      domain.TaskID
		displayName string
		state       domain.TaskState
	}
	rows, err := tx.QueryContext(ctx, `
SELECT t.id, a.display_name, t.state
FROM tasks t
JOIN agents a ON a.id = t.target_agent_id
WHERE t.parent_task_id = ? AND t.title NOT LIKE ?
ORDER BY t.created_at, t.id`, parentTaskID, agentCallbackTitlePrefix+"%")
	if err != nil {
		return "", err
	}
	results := make([]result, 0)
	for rows.Next() {
		var item result
		if err := rows.Scan(&item.taskID, &item.displayName, &item.state); err != nil {
			rows.Close()
			return "", err
		}
		results = append(results, item)
	}
	if err := rows.Close(); err != nil {
		return "", err
	}

	var builder strings.Builder
	builder.WriteString("所有直接专家任务已经结束。请基于下列全部结果给用户做一次最终汇总，不要把某一位专家的回答冒充为全员结论。")
	for _, item := range results {
		var body string
		_ = tx.QueryRowContext(ctx, `
SELECT body
FROM messages
WHERE json_extract(metadata_json, '$.task_id') = ?
ORDER BY sequence DESC
LIMIT 1`, item.taskID).Scan(&body)
		body = strings.TrimSpace(body)
		if marker := strings.Index(body, "\n\n执行凭证\n"); marker >= 0 {
			body = strings.TrimSpace(body[:marker])
		}
		if marker := strings.Index(body, "\n\n"); marker >= 0 && (strings.HasPrefix(body, "交付：") || strings.HasPrefix(body, "任务失败：")) {
			body = strings.TrimSpace(body[marker+2:])
		}
		if body == "" {
			body = "未返回可见文本"
		}
		if len(body) > 6000 {
			body = body[:6000] + "\n[单位专家输出过长，已截断]"
		}
		builder.WriteString("\n\n## ")
		builder.WriteString(item.displayName)
		builder.WriteString("（状态：")
		builder.WriteString(string(item.state))
		builder.WriteString("）\n")
		builder.WriteString(body)
	}
	return builder.String(), nil
}

func (r *CollabRepository) CompleteTask(ctx context.Context, runnerID domain.RunnerID, taskID domain.TaskID, success bool, output, errorSummary string, exitCode int, receipt domain.ExecutionReceipt) (domain.Message, error) {
	var message domain.Message
	now := time.Now().UTC()
	err := r.store.Write(ctx, func(tx *sql.Tx) error {
		var workspaceID domain.WorkspaceID
		var roomID domain.RoomID
		var agentID domain.AgentID
		var title, deviceName, workflowRole string
		var attempt int
		var creatorKind domain.TaskCreatorKind
		var executionSource domain.TaskExecutionSource
		var creatorID string
		var workItemID, parentTaskID, rootMessageID, sourceMessageID sql.NullString
		var collaborationDepth int
		var currentState domain.TaskState
		if err := tx.QueryRowContext(ctx, `
SELECT t.workspace_id, t.room_id, t.target_agent_id, t.title, d.name, t.current_attempt, t.creator_kind, t.creator_id, t.work_item_id, t.parent_task_id,
       t.root_message_id, t.source_message_id, t.collaboration_depth, t.state, t.workflow_role, t.execution_source
FROM tasks t
JOIN agents a ON a.id = t.target_agent_id
JOIN runners r ON r.id = a.runner_id
JOIN devices d ON d.id = r.device_id
WHERE t.id = ? AND a.runner_id = ?`, taskID, runnerID).Scan(
			&workspaceID, &roomID, &agentID, &title, &deviceName, &attempt, &creatorKind, &creatorID, &workItemID, &parentTaskID, &rootMessageID, &sourceMessageID, &collaborationDepth, &currentState, &workflowRole, &executionSource,
		); err != nil {
			return mapNotFound(err)
		}
		// Completion is an idempotent acknowledgement. A Runner can lose the
		// HTTP response after the Hub committed the delivery; retrying must
		// return the existing visible message instead of creating a duplicate.
		if currentState == domain.TaskDelivered || currentState == domain.TaskFailed {
			existing, err := scanMessage(tx.QueryRowContext(ctx, `
SELECT message_id, workspace_id, room_id, sequence, sender_kind, sender_id, kind, body,
       mention_agent_ids_json, reply_to_message_id, root_message_id, metadata_json, created_at
FROM messages
WHERE json_extract(metadata_json, '$.task_id') = ?
ORDER BY sequence DESC LIMIT 1`, taskID))
			if err != nil {
				return mapNotFound(err)
			}
			message = existing
			return nil
		}
		if currentState != domain.TaskDispatched && currentState != domain.TaskRunning {
			return auth.ErrConflict
		}
		state := domain.TaskFailed
		kind := domain.MessageProgress
		rawBody := strings.TrimSpace(errorSummary)
		if success {
			state = domain.TaskDelivered
			kind = domain.MessageDelivery
			rawBody = strings.TrimSpace(output)
		}
		if rawBody == "" {
			rawBody = "任务已完成，但 Agent 没有返回文本。"
		}
		if len(rawBody) > 19_000 {
			rawBody = rawBody[:19_000] + "\n\n[输出过长，已截断]"
		}
		body := rawBody
		if success {
			body = "交付：" + title + "\n\n" + body
		} else {
			body = "任务失败：" + title + "\n\n" + body
		}
		receiptJSON, _ := json.Marshal(receipt)
		if strings.TrimSpace(receipt.Executable) != "" {
			body += fmt.Sprintf("\n\n执行凭证\n执行设备：%s\n原生 Agent：%s\nCLI：%s\n版本：%s\n进程 PID：%d\n开始：%s\n结束：%s", deviceName, receipt.NativeAgentType, receipt.Executable, receipt.Version, receipt.PID, receipt.StartedAt, receipt.FinishedAt)
			if receipt.SkillCount > 0 || receipt.ResourceCount > 0 {
				body += fmt.Sprintf("\n本机能力上下文：%d Skills · %d Resources", receipt.SkillCount, receipt.ResourceCount)
			}
			if len(receipt.MatchedSkills) > 0 {
				body += "\n任务/交付提及 Skill：" + strings.Join(receipt.MatchedSkills, "、")
			}
			if len(receipt.MatchedResources) > 0 {
				body += "\n任务/交付提及资源：" + strings.Join(receipt.MatchedResources, "、")
			}
			if receipt.NativeSessionID != "" {
				body += "\n原生会话 ID：" + receipt.NativeSessionID
			}
			if receipt.NativeHistory != "" {
				body += "\n原软件历史：" + receipt.NativeHistory
			}
			if receipt.ProjectCommit != "" {
				body += "\n项目步骤：" + receipt.ProjectStepID + "\n基准版本：" + receipt.ProjectCommit
			}
			if receipt.ExecutionSource == string(domain.TaskExecutionResponsibleLocal) {
				body += "\n执行依据：负责人本机最新工作目录"
				if receipt.LocalDirectoryLabel != "" {
					body += "（" + receipt.LocalDirectoryLabel + "）"
				}
				body += "\n版本状态：仅汇报实时进展，尚未提交审查或合并"
			} else if receipt.ExecutionSource == string(domain.TaskExecutionOfficial) || (receipt.ExecutionSource == "" && receipt.ProjectCommit != "") {
				body += "\n执行依据：团队正式版本"
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE tasks SET state = ?, updated_at = ? WHERE id = ?`, state, formatTime(now), taskID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE task_attempts SET state = ?, finished_at = ?, exit_code = ?, error_summary = ?, execution_receipt_json = ?
WHERE task_id = ? AND attempt_no = ? AND runner_id = ?`, state, formatTime(now), exitCode, errorSummary, string(receiptJSON), taskID, attempt, runnerID); err != nil {
			return err
		}
		if workItemID.Valid {
			if workflowRole == "ai_merge_review" {
				status := domain.WorkItemAIReviewFailed
				if success {
					if strings.Contains(rawBody, "建议合并") && !strings.Contains(rawBody, "不建议合并") && !strings.Contains(rawBody, "建议修改后再合并") {
						status = domain.WorkItemAIReviewPassed
					} else {
						status = domain.WorkItemAIReviewChangesRequested
					}
				}
				if _, err := tx.ExecContext(ctx, `UPDATE work_items SET ai_review_status = ?, ai_review_summary = ?, updated_at = ? WHERE id = ? AND review_task_id = ?`, status, rawBody, formatTime(now), workItemID.String, taskID); err != nil {
					return err
				}
			}
			if workflowRole == "post_merge_verification" {
				status := domain.WorkItemVerificationFailed
				itemStatus := domain.WorkItemReview
				var acceptedAt any
				if success && strings.Contains(rawBody, "校验结论：通过") && !strings.Contains(rawBody, "校验结论：未通过") {
					status = domain.WorkItemVerificationPassed
					itemStatus = domain.WorkItemDone
					acceptedAt = formatTime(now)
				}
				if _, err := tx.ExecContext(ctx, `UPDATE work_items SET verification_status = ?, verification_summary = ?, status = ?, accepted_at = ?, updated_at = ? WHERE id = ? AND verification_task_id = ?`, status, rawBody, itemStatus, acceptedAt, formatTime(now), workItemID.String, taskID); err != nil {
					return err
				}
			}
			if err := syncWorkItemStateTx(ctx, tx, domain.WorkItemID(workItemID.String), now); err != nil {
				return err
			}
		}
		if success && strings.TrimSpace(receipt.NativeSessionID) != "" {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO agent_sessions (room_id, agent_id, native_session_id, state, last_task_id, updated_at)
VALUES (?, ?, ?, 'active', ?, ?)
ON CONFLICT(room_id, agent_id) DO UPDATE SET native_session_id = excluded.native_session_id, state = 'active',
    last_task_id = excluded.last_task_id, updated_at = excluded.updated_at`, roomID, agentID, receipt.NativeSessionID, taskID, formatTime(now)); err != nil {
				return err
			}
		}
		messageID, err := idgen.New("msg")
		if err != nil {
			return err
		}
		sequence, err := allocateSequence(ctx, tx, workspaceID)
		if err != nil {
			return err
		}
		root := domain.MessageID(messageID)
		if rootMessageID.Valid {
			root = domain.MessageID(rootMessageID.String)
		} else if sourceMessageID.Valid {
			root = domain.MessageID(sourceMessageID.String)
		}
		var rootUserID domain.UserID
		if rootMessageID.Valid || sourceMessageID.Valid {
			if err := tx.QueryRowContext(ctx, `SELECT sender_id FROM messages WHERE message_id = ? AND sender_kind = 'user'`, root).Scan(&rootUserID); err != nil {
				return mapNotFound(err)
			}
		} else if creatorKind == domain.TaskCreatorUser {
			rootUserID = domain.UserID(creatorID)
		}
		agents, err := roomRoutingAgentsTx(ctx, tx, roomID)
		if err != nil {
			return err
		}
		agents = invokableAgents(rootUserID, agents)
		mentions := []domain.AgentID{}
		if success {
			mentions = mentionedAgentIDs(rawBody, agents)
		}
		mentionJSON, _ := json.Marshal(mentions)
		var replyTo *domain.MessageID
		if sourceMessageID.Valid {
			value := domain.MessageID(sourceMessageID.String)
			replyTo = &value
		}
		metadataJSON, _ := json.Marshal(map[string]any{"task_id": taskID, "task_state": state})
		message = domain.Message{ID: domain.MessageID(messageID), WorkspaceID: workspaceID, RoomID: roomID, Sequence: sequence,
			SenderKind: domain.MessageSenderAgent, SenderID: string(agentID), Kind: kind, Body: body, MentionAgentIDs: mentions,
			ReplyToMessageID: replyTo, RootMessageID: &root, Metadata: metadataJSON, CreatedAt: now}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO messages (message_id, workspace_id, room_id, sequence, sender_kind, sender_id, kind, body,
                      mention_agent_ids_json, reply_to_message_id, root_message_id, metadata_json, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, message.ID, message.WorkspaceID, message.RoomID, message.Sequence,
			message.SenderKind, message.SenderID, message.Kind, message.Body, string(mentionJSON), replyTo, root, string(metadataJSON), formatTime(message.CreatedAt)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE message_attachments SET message_id = ? WHERE task_id = ? AND message_id IS NULL`, message.ID, taskID); err != nil {
			return err
		}
		message.Attachments, err = listAttachmentsTx(ctx, tx, message.ID)
		if err != nil {
			return err
		}
		routingTargets := append([]domain.AgentID{}, mentions...)
		routingSources := make(map[domain.AgentID]string, len(routingTargets))
		for _, mentionedID := range routingTargets {
			routingSources[mentionedID] = rawBody
		}
		var currentAgent routingAgent
		for _, candidate := range agents {
			if candidate.ID == agentID {
				currentAgent = candidate
				break
			}
		}
		if success && creatorKind == domain.TaskCreatorUser && collaborationDepth == 0 && currentAgent.ResponseMode == domain.AgentResponseCoordinator {
			var rootBody string
			if err := tx.QueryRowContext(ctx, `SELECT body FROM messages WHERE message_id = ?`, root).Scan(&rootBody); err != nil {
				return err
			}
			if requestsCoordinatorFanout(rootBody) {
				for _, candidate := range agents {
					if candidate.ID == agentID || candidate.ResponseMode == domain.AgentResponseMuted {
						continue
					}
					alreadyIncluded := false
					for _, id := range routingTargets {
						alreadyIncluded = alreadyIncluded || id == candidate.ID
					}
					if !alreadyIncluded {
						routingTargets = append(routingTargets, candidate.ID)
						routingSources[candidate.ID] = "用户原始要求：\n" + rootBody + "\n\n主协调的开场与已有分工：\n" + rawBody + "\n\n主协调要求召集全员。请从你最擅长的独立视角完成原始任务，给出明确结论、核心依据和可执行建议，不要只汇报计划。"
					}
				}
			}
		}
		if success && len(routingTargets) > 0 {
			handoffSource := executionSource
			if handoffSource == domain.TaskExecutionResponsibleLocal {
				handoffSource = domain.TaskExecutionNone
			}
			var agentTaskCount int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE root_message_id = ? AND creator_kind = 'agent'`, root).Scan(&agentTaskCount); err != nil {
				return err
			}
			for _, mentionedID := range routingTargets {
				if mentionedID == agentID || agentTaskCount >= maxAgentTasksPerRoot {
					continue
				}
				var target routingAgent
				found := false
				for _, candidate := range agents {
					if candidate.ID == mentionedID {
						target, found = candidate, true
						break
					}
				}
				nextDepth := collaborationDepth + 1
				if !found || target.ResponseMode == domain.AgentResponseMuted || nextDepth > target.MaxAutoTurns {
					continue
				}
				parent := taskID
				if err := createRoutedTaskTx(ctx, tx, routedTaskInput{WorkspaceID: workspaceID, RoomID: roomID,
					RootMessageID: root, SourceMessageID: message.ID, ParentTaskID: &parent, CreatorKind: domain.TaskCreatorAgent,
					CreatorID: string(agentID), Target: target, SourceBody: routingSources[mentionedID], Depth: nextDepth, CreatedAt: now, ExecutionSource: handoffSource}, agents); err != nil {
					return err
				}
				agentTaskCount++
			}
		}
		if creatorKind == domain.TaskCreatorAgent && creatorID != "" && creatorID != string(agentID) && !strings.HasPrefix(title, agentCallbackTitlePrefix) && len(mentions) == 0 && parentTaskID.Valid {
			callbackSource := executionSource
			if callbackSource == domain.TaskExecutionResponsibleLocal {
				callbackSource = domain.TaskExecutionNone
			}
			batchParent := domain.TaskID(parentTaskID.String)
			ready, err := directCollaborationBatchReadyTx(ctx, tx, batchParent)
			if err != nil {
				return err
			}
			creatorAgentID := domain.AgentID(creatorID)
			exists, err := collaborationCallbackExistsTx(ctx, tx, batchParent, creatorAgentID)
			if err != nil {
				return err
			}
			if ready && !exists {
				var target routingAgent
				found := false
				for _, candidate := range agents {
					if candidate.ID == creatorAgentID {
						target, found = candidate, true
						break
					}
				}
				nextDepth := collaborationDepth + 1
				if found && target.ResponseMode != domain.AgentResponseMuted && nextDepth <= target.MaxAutoTurns {
					summary, err := collaborationBatchSummaryTx(ctx, tx, batchParent)
					if err != nil {
						return err
					}
					parent := batchParent
					if err := createRoutedTaskTx(ctx, tx, routedTaskInput{WorkspaceID: workspaceID, RoomID: roomID,
						RootMessageID: root, SourceMessageID: message.ID, ParentTaskID: &parent, CreatorKind: domain.TaskCreatorAgent,
						CreatorID: string(agentID), Target: target, SourceBody: summary, Depth: nextDepth, CreatedAt: now, IsCallback: true, ExecutionSource: callbackSource}, agents); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	return message, err
}

func allocateSequence(ctx context.Context, tx *sql.Tx, workspaceID domain.WorkspaceID) (uint64, error) {
	var sequence uint64
	if err := tx.QueryRowContext(ctx, `
UPDATE event_sequence SET next_sequence = next_sequence + 1
WHERE workspace_id = ? RETURNING next_sequence - 1`, workspaceID).Scan(&sequence); err != nil {
		return 0, fmt.Errorf("allocate workspace sequence: %w", err)
	}
	return sequence, nil
}
