package collaboration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/trees"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDatabaseUnavailable = errors.New("collaboration database is unavailable")
	ErrNotFound            = errors.New("collaboration resource not found")
	ErrForbidden           = errors.New("collaboration access is forbidden")
	ErrValidation          = errors.New("collaboration input is invalid")
	ErrConflict            = errors.New("collaboration state conflict")
	ErrInvitationExpired   = errors.New("invitation has expired")
)

type InviteInput struct {
	InviteeUserID   string `json:"invitee_user_id"`
	InviteeEmail    string `json:"invitee_email"`
	PermissionLevel string `json:"permission_level"`
}

type UpdatePermissionInput struct {
	PermissionLevel string `json:"permission_level"`
}

type InviteeView struct {
	UserID          string    `json:"userId,omitempty"`
	DisplayName     string    `json:"displayName"`
	Email           string    `json:"email,omitempty"`
	PermissionLevel string    `json:"permissionLevel"`
	InvitedBy       string    `json:"invitedBy,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
}

type InvitationView struct {
	ID              string     `json:"id"`
	TreeID          string     `json:"treeId"`
	TreeName        string     `json:"treeName"`
	InviteeUserID   string     `json:"inviteeUserId,omitempty"`
	InviteeEmail    string     `json:"inviteeEmail,omitempty"`
	PermissionLevel string     `json:"permissionLevel"`
	Status          string     `json:"status"`
	ExpiresAt       time.Time  `json:"expiresAt"`
	AcceptedAt      *time.Time `json:"acceptedAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
}

type InvitationCreated struct {
	Invitation  InvitationView `json:"invitation"`
	AcceptToken string         `json:"acceptToken"`
	AcceptPath  string         `json:"acceptPath"`
}

type CollaboratorsResponse struct {
	Owner         InviteeView      `json:"owner"`
	Collaborators []InviteeView    `json:"collaborators"`
	Invitations   []InvitationView `json:"invitations"`
	CanManage     bool             `json:"canManage"`
}

type ActivityView struct {
	ID         string          `json:"id"`
	Action     string          `json:"action"`
	EntityType string          `json:"entityType"`
	EntityID   string          `json:"entityId"`
	ActorID    string          `json:"actorId,omitempty"`
	ActorName  string          `json:"actorName,omitempty"`
	Before     json.RawMessage `json:"before,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
	ReasonAR   string          `json:"reasonAr,omitempty"`
	CreatedAt  time.Time       `json:"createdAt"`
}

type AcceptResult struct {
	InvitationID    string `json:"invitationId"`
	TreeID          string `json:"treeId"`
	TreeName        string `json:"treeName"`
	PermissionLevel string `json:"permissionLevel"`
}

type Service struct {
	Pool  *pgxpool.Pool
	Trees *trees.Service
}

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{Pool: pool, Trees: trees.NewService(pool)}
}

func (s *Service) ListCollaborators(ctx context.Context, treeID, viewerID string) (CollaboratorsResponse, error) {
	if err := s.ready(); err != nil {
		return CollaboratorsResponse{}, err
	}
	treeUUID, viewerUUID, err := parseIDs(treeID, viewerID)
	if err != nil {
		return CollaboratorsResponse{}, err
	}
	ownerID, _, err := s.loadTree(ctx, s.Pool, treeUUID)
	if err != nil {
		return CollaboratorsResponse{}, err
	}
	canView, err := s.Trees.CanViewDraft(ctx, treeUUID.String(), viewerUUID.String())
	if err != nil {
		return CollaboratorsResponse{}, err
	}
	if !canView {
		return CollaboratorsResponse{}, ErrForbidden
	}
	canManage := ownerID == viewerUUID
	var owner InviteeView
	if err := s.Pool.QueryRow(ctx, `
		SELECT u.id, u.display_name_ar, u.email, t.created_at
		FROM trees t
		JOIN users u ON u.id = t.owner_id
		WHERE t.id = $1
	`, treeUUID).Scan(&owner.UserID, &owner.DisplayName, &owner.Email, &owner.CreatedAt); err != nil {
		return CollaboratorsResponse{}, err
	}
	owner.PermissionLevel = "owner"
	if !canManage {
		owner.Email = ""
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT tc.user_id, u.display_name_ar, u.email, tc.permission_level, inviter.display_name_ar, tc.created_at
		FROM tree_collaborators tc
		JOIN users u ON u.id = tc.user_id
		JOIN users inviter ON inviter.id = tc.invited_by
		WHERE tc.tree_id = $1
		ORDER BY u.display_name_ar, u.email
	`, treeUUID)
	if err != nil {
		return CollaboratorsResponse{}, err
	}
	defer rows.Close()
	collaborators := make([]InviteeView, 0)
	for rows.Next() {
		var item InviteeView
		if err := rows.Scan(&item.UserID, &item.DisplayName, &item.Email, &item.PermissionLevel, &item.InvitedBy, &item.CreatedAt); err != nil {
			return CollaboratorsResponse{}, err
		}
		if !canManage {
			item.Email = ""
		}
		collaborators = append(collaborators, item)
	}
	if err := rows.Err(); err != nil {
		return CollaboratorsResponse{}, err
	}
	invitations := make([]InvitationView, 0)
	if canManage {
		if _, err := s.Pool.Exec(ctx, `
			UPDATE tree_invitations
			SET status = 'expired'
			WHERE tree_id = $1 AND status = 'pending' AND expires_at <= now()
		`, treeUUID); err != nil {
			return CollaboratorsResponse{}, err
		}
		rows, err := s.Pool.Query(ctx, `
			SELECT ti.id, ti.tree_id, t.name_ar, ti.invitee_user_id, ti.invitee_email,
			       ti.permission_level, ti.status, ti.expires_at, ti.accepted_at, ti.created_at
			FROM tree_invitations ti
			JOIN trees t ON t.id = ti.tree_id
			WHERE ti.tree_id = $1 AND ti.status = 'pending'
			ORDER BY ti.created_at DESC
		`, treeUUID)
		if err != nil {
			return CollaboratorsResponse{}, err
		}
		for rows.Next() {
			item, err := scanInvitation(rows)
			if err != nil {
				rows.Close()
				return CollaboratorsResponse{}, err
			}
			invitations = append(invitations, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return CollaboratorsResponse{}, err
		}
		rows.Close()
	}
	return CollaboratorsResponse{
		Owner:         owner,
		Collaborators: collaborators,
		Invitations:   invitations,
		CanManage:     canManage,
	}, nil
}

func (s *Service) CreateInvitation(ctx context.Context, treeID, inviterID string, input InviteInput) (InvitationCreated, error) {
	if err := s.ready(); err != nil {
		return InvitationCreated{}, err
	}
	treeUUID, inviterUUID, err := parseIDs(treeID, inviterID)
	if err != nil {
		return InvitationCreated{}, err
	}
	input, err = validateInviteInput(input)
	if err != nil {
		return InvitationCreated{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return InvitationCreated{}, err
	}
	defer tx.Rollback(ctx)
	var ownerID uuid.UUID
	var treeName string
	if err := tx.QueryRow(ctx, `SELECT owner_id, name_ar FROM trees WHERE id = $1 FOR UPDATE`, treeUUID).Scan(&ownerID, &treeName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return InvitationCreated{}, ErrNotFound
		}
		return InvitationCreated{}, err
	}
	if ownerID != inviterUUID {
		return InvitationCreated{}, ErrForbidden
	}
	var inviteeUserID pgtype.UUID
	var inviteeEmail pgtype.Text
	if input.InviteeUserID != "" {
		inviteeUserID, inviteeEmail, err = s.loadTargetUser(ctx, tx, input.InviteeUserID)
	} else {
		inviteeEmail = pgtype.Text{String: input.InviteeEmail, Valid: true}
		var foundID pgtype.UUID
		err = tx.QueryRow(ctx, `
			SELECT id, email
			FROM users
			WHERE lower(email) = $1 AND status = 'active'
		`, input.InviteeEmail).Scan(&foundID, &inviteeEmail)
		if errors.Is(err, pgx.ErrNoRows) {
			err = nil
		}
		if err == nil && foundID.Valid {
			inviteeUserID = foundID
		}
	}
	if err != nil {
		return InvitationCreated{}, err
	}
	if inviteeUserID.Valid && uuid.UUID(inviteeUserID.Bytes) == ownerID {
		return InvitationCreated{}, ErrConflict
	}
	if inviteeUserID.Valid {
		var member bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM tree_collaborators WHERE tree_id = $1 AND user_id = $2)
		`, treeUUID, uuid.UUID(inviteeUserID.Bytes)).Scan(&member); err != nil {
			return InvitationCreated{}, err
		}
		if member {
			return InvitationCreated{}, ErrConflict
		}
	}
	if inviteeEmail.Valid && strings.EqualFold(inviteeEmail.String, "") {
		return InvitationCreated{}, ErrValidation
	}
	if _, err := tx.Exec(ctx, `
		UPDATE tree_invitations
		SET status = 'expired'
		WHERE tree_id = $1 AND status = 'pending' AND expires_at <= now()
	`, treeUUID); err != nil {
		return InvitationCreated{}, err
	}
	var pending bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM tree_invitations
			WHERE tree_id = $1
			  AND status = 'pending'
			  AND (
				($2::uuid IS NOT NULL AND invitee_user_id = $2)
				OR ($3::text IS NOT NULL AND lower(invitee_email) = $3)
			  )
		)
	`, treeUUID, nullableUUID(inviteeUserID), nullableText(inviteeEmail)).Scan(&pending); err != nil {
		return InvitationCreated{}, err
	}
	if pending {
		return InvitationCreated{}, ErrConflict
	}
	token, err := auth.NewSessionToken()
	if err != nil {
		return InvitationCreated{}, err
	}
	invitationID := uuid.New()
	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour)
	if _, err := tx.Exec(ctx, `
		INSERT INTO tree_invitations
			(id, tree_id, inviter_id, invitee_user_id, invitee_email, token_hash, permission_level, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, invitationID, treeUUID, inviterUUID, nullableUUID(inviteeUserID), nullableText(inviteeEmail), auth.HashToken(token), input.PermissionLevel, expiresAt); err != nil {
		return InvitationCreated{}, err
	}
	after := map[string]any{
		"invitee_user_id":  inviteeUserIDString(inviteeUserID),
		"invitee_email":    inviteeText(inviteeEmail),
		"permission_level": input.PermissionLevel,
	}
	if err := writeLifecycle(ctx, tx, treeUUID, nil, inviterUUID, "collaborator_invited", "tree_invitation", invitationID, nil, after, ""); err != nil {
		return InvitationCreated{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return InvitationCreated{}, err
	}
	return InvitationCreated{
		Invitation: InvitationView{
			ID:              invitationID.String(),
			TreeID:          treeUUID.String(),
			TreeName:        treeName,
			InviteeUserID:   inviteeUserIDString(inviteeUserID),
			InviteeEmail:    inviteeText(inviteeEmail),
			PermissionLevel: input.PermissionLevel,
			Status:          "pending",
			ExpiresAt:       expiresAt,
			CreatedAt:       time.Now().UTC(),
		},
		AcceptToken: token,
		AcceptPath:  fmt.Sprintf("/invitation/%s", token),
	}, nil
}

func (s *Service) ListInvitations(ctx context.Context, userID string) ([]InvitationView, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, ErrForbidden
	}
	var email string
	if err := s.Pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1 AND status = 'active'`, userUUID).Scan(&email); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrForbidden
		}
		return nil, err
	}
	if _, err := s.Pool.Exec(ctx, `
		UPDATE tree_invitations
		SET status = 'expired'
		WHERE status = 'pending' AND expires_at <= now()
		  AND (invitee_user_id = $1 OR lower(invitee_email) = lower($2))
	`, userUUID, email); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT ti.id, ti.tree_id, t.name_ar, ti.invitee_user_id, ti.invitee_email,
		       ti.permission_level, ti.status, ti.expires_at, ti.accepted_at, ti.created_at
		FROM tree_invitations ti
		JOIN trees t ON t.id = ti.tree_id
		WHERE (ti.invitee_user_id = $1 OR lower(ti.invitee_email) = lower($2))
		  AND ti.status = 'pending'
		ORDER BY ti.created_at DESC
	`, userUUID, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]InvitationView, 0)
	for rows.Next() {
		item, err := scanInvitation(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) AcceptInvitation(ctx context.Context, token, userID string) (AcceptResult, error) {
	if err := s.ready(); err != nil {
		return AcceptResult{}, err
	}
	if strings.TrimSpace(token) == "" {
		return AcceptResult{}, ErrNotFound
	}
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return AcceptResult{}, ErrForbidden
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return AcceptResult{}, err
	}
	defer tx.Rollback(ctx)
	var invitationID, treeID, inviterID uuid.UUID
	var inviteeUserID pgtype.UUID
	var inviteeEmail pgtype.Text
	var permissionLevel, status string
	var expiresAt time.Time
	var acceptedAt pgtype.Timestamptz
	if err := tx.QueryRow(ctx, `
		SELECT id, tree_id, inviter_id, invitee_user_id, invitee_email,
		       permission_level, status, expires_at, accepted_at
		FROM tree_invitations
		WHERE token_hash = $1
		FOR UPDATE
	`, auth.HashToken(token)).Scan(&invitationID, &treeID, &inviterID, &inviteeUserID, &inviteeEmail, &permissionLevel, &status, &expiresAt, &acceptedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AcceptResult{}, ErrNotFound
		}
		return AcceptResult{}, err
	}
	if status != "pending" {
		return AcceptResult{}, ErrConflict
	}
	if !expiresAt.After(time.Now().UTC()) {
		if _, err := tx.Exec(ctx, `UPDATE tree_invitations SET status = 'expired' WHERE id = $1`, invitationID); err != nil {
			return AcceptResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return AcceptResult{}, err
		}
		return AcceptResult{}, ErrInvitationExpired
	}
	var actorEmail string
	if err := tx.QueryRow(ctx, `SELECT email FROM users WHERE id = $1 AND status = 'active'`, userUUID).Scan(&actorEmail); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AcceptResult{}, ErrForbidden
		}
		return AcceptResult{}, err
	}
	if inviteeUserID.Valid && uuid.UUID(inviteeUserID.Bytes) != userUUID {
		return AcceptResult{}, ErrNotFound
	}
	if !inviteeUserID.Valid && (!inviteeEmail.Valid || !strings.EqualFold(inviteeEmail.String, actorEmail)) {
		return AcceptResult{}, ErrNotFound
	}
	var member bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM tree_collaborators WHERE tree_id = $1 AND user_id = $2)
	`, treeID, userUUID).Scan(&member); err != nil {
		return AcceptResult{}, err
	}
	if member {
		return AcceptResult{}, ErrConflict
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tree_collaborators (tree_id, user_id, permission_level, invited_by)
		VALUES ($1, $2, $3, $4)
	`, treeID, userUUID, permissionLevel, inviterID); err != nil {
		return AcceptResult{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE tree_invitations
		SET status = 'accepted', accepted_at = now()
		WHERE id = $1
	`, invitationID); err != nil {
		return AcceptResult{}, err
	}
	var treeName string
	if err := tx.QueryRow(ctx, `SELECT name_ar FROM trees WHERE id = $1`, treeID).Scan(&treeName); err != nil {
		return AcceptResult{}, err
	}
	after := map[string]any{"user_id": userUUID.String(), "permission_level": permissionLevel}
	if err := writeLifecycle(ctx, tx, treeID, nil, userUUID, "invitation_accepted", "tree_collaborator", userUUID, nil, after, ""); err != nil {
		return AcceptResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AcceptResult{}, err
	}
	return AcceptResult{InvitationID: invitationID.String(), TreeID: treeID.String(), TreeName: treeName, PermissionLevel: permissionLevel}, nil
}

func (s *Service) UpdateCollaborator(ctx context.Context, treeID, collaboratorID, actorID string, input UpdatePermissionInput) (InviteeView, error) {
	if err := s.ready(); err != nil {
		return InviteeView{}, err
	}
	treeUUID, actorUUID, err := parseIDs(treeID, actorID)
	if err != nil {
		return InviteeView{}, err
	}
	collaboratorTarget, err := uuid.Parse(collaboratorID)
	if err != nil {
		return InviteeView{}, ErrNotFound
	}
	input.PermissionLevel = strings.TrimSpace(input.PermissionLevel)
	if !validPermissionLevel(input.PermissionLevel) {
		return InviteeView{}, ErrValidation
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return InviteeView{}, err
	}
	defer tx.Rollback(ctx)
	var ownerID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT owner_id FROM trees WHERE id = $1 FOR UPDATE`, treeUUID).Scan(&ownerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return InviteeView{}, ErrNotFound
		}
		return InviteeView{}, err
	}
	if ownerID != actorUUID {
		return InviteeView{}, ErrForbidden
	}
	if collaboratorTarget == ownerID {
		return InviteeView{}, ErrConflict
	}
	var current InviteeView
	if err := tx.QueryRow(ctx, `
		SELECT tc.user_id, u.display_name_ar, u.email, tc.permission_level, inviter.display_name_ar, tc.created_at
		FROM tree_collaborators tc
		JOIN users u ON u.id = tc.user_id
		JOIN users inviter ON inviter.id = tc.invited_by
		WHERE tc.tree_id = $1 AND tc.user_id = $2
		FOR UPDATE
	`, treeUUID, collaboratorTarget).Scan(&current.UserID, &current.DisplayName, &current.Email, &current.PermissionLevel, &current.InvitedBy, &current.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return InviteeView{}, ErrNotFound
		}
		return InviteeView{}, err
	}
	if current.PermissionLevel == input.PermissionLevel {
		if err := tx.Commit(ctx); err != nil {
			return InviteeView{}, err
		}
		return current, nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE tree_collaborators SET permission_level = $1
		WHERE tree_id = $2 AND user_id = $3
	`, input.PermissionLevel, treeUUID, collaboratorTarget); err != nil {
		return InviteeView{}, err
	}
	after := map[string]any{"user_id": collaboratorTarget.String(), "permission_level": input.PermissionLevel}
	if err := writeLifecycle(ctx, tx, treeUUID, nil, actorUUID, "collaborator_permission_changed", "tree_collaborator", collaboratorTarget, map[string]any{"permission_level": current.PermissionLevel}, after, ""); err != nil {
		return InviteeView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return InviteeView{}, err
	}
	current.PermissionLevel = input.PermissionLevel
	return current, nil
}

func (s *Service) RemoveCollaborator(ctx context.Context, treeID, collaboratorID, actorID string) error {
	if err := s.ready(); err != nil {
		return err
	}
	treeUUID, actorUUID, err := parseIDs(treeID, actorID)
	if err != nil {
		return err
	}
	collaboratorTarget, err := uuid.Parse(collaboratorID)
	if err != nil {
		return ErrNotFound
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var ownerID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT owner_id FROM trees WHERE id = $1 FOR UPDATE`, treeUUID).Scan(&ownerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if ownerID != actorUUID {
		return ErrForbidden
	}
	if collaboratorTarget == ownerID {
		return ErrConflict
	}
	var removedID pgtype.UUID
	if err := tx.QueryRow(ctx, `
		SELECT user_id FROM tree_collaborators
		WHERE tree_id = $1 AND user_id = $2
		FOR UPDATE
	`, treeUUID, collaboratorTarget).Scan(&removedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM tree_collaborators WHERE tree_id = $1 AND user_id = $2`, treeUUID, collaboratorTarget); err != nil {
		return err
	}
	if err := writeLifecycle(ctx, tx, treeUUID, nil, actorUUID, "collaborator_removed", "tree_collaborator", collaboratorTarget, map[string]any{"user_id": collaboratorTarget.String()}, nil, ""); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Service) RevokeInvitation(ctx context.Context, treeID, invitationID, actorID string) error {
	if err := s.ready(); err != nil {
		return err
	}
	treeUUID, actorUUID, err := parseIDs(treeID, actorID)
	if err != nil {
		return err
	}
	inviteUUID, err := uuid.Parse(invitationID)
	if err != nil {
		return ErrNotFound
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var ownerID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT owner_id FROM trees WHERE id = $1 FOR UPDATE`, treeUUID).Scan(&ownerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if ownerID != actorUUID {
		return ErrForbidden
	}
	var status string
	if err := tx.QueryRow(ctx, `
		SELECT status FROM tree_invitations
		WHERE id = $1 AND tree_id = $2
		FOR UPDATE
	`, inviteUUID, treeUUID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if status != "pending" {
		return ErrConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE tree_invitations SET status = 'revoked' WHERE id = $1`, inviteUUID); err != nil {
		return err
	}
	if err := writeLifecycle(ctx, tx, treeUUID, nil, actorUUID, "invitation_revoked", "tree_invitation", inviteUUID, map[string]any{"status": "pending"}, map[string]any{"status": "revoked"}, ""); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Service) ListActivity(ctx context.Context, treeID, viewerID string) ([]ActivityView, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	treeUUID, viewerUUID, err := parseIDs(treeID, viewerID)
	if err != nil {
		return nil, err
	}
	if _, _, err := s.loadTree(ctx, s.Pool, treeUUID); err != nil {
		return nil, err
	}
	canView, err := s.Trees.CanViewDraft(ctx, treeUUID.String(), viewerUUID.String())
	if err != nil {
		return nil, err
	}
	if !canView {
		return nil, ErrForbidden
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT c.id::text, c.action, c.entity_type, c.entity_id::text,
		       COALESCE(c.actor_id::text, ''), COALESCE(u.display_name_ar, ''),
		       COALESCE(c.before_value::text, ''), COALESCE(c.after_value::text, ''),
		       COALESCE(c.reason_ar, ''), c.created_at
		FROM tree_change_log c
		LEFT JOIN users u ON u.id = c.actor_id
		WHERE c.tree_id = $1
		ORDER BY c.created_at DESC
		LIMIT 50
	`, treeUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ActivityView, 0)
	for rows.Next() {
		var item ActivityView
		var actorID, actorName, before, after, reason string
		if err := rows.Scan(&item.ID, &item.Action, &item.EntityType, &item.EntityID, &actorID, &actorName, &before, &after, &reason, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.ActorID = actorID
		item.ActorName = actorName
		item.ReasonAR = reason
		if before != "" {
			item.Before = json.RawMessage(before)
		}
		if after != "" {
			item.After = json.RawMessage(after)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) ready() error {
	if s == nil || s.Pool == nil || s.Trees == nil {
		return ErrDatabaseUnavailable
	}
	return nil
}

func (s *Service) loadTree(ctx context.Context, q queryer, treeID uuid.UUID) (uuid.UUID, string, error) {
	var ownerID uuid.UUID
	var name string
	if err := q.QueryRow(ctx, `SELECT owner_id, name_ar FROM trees WHERE id = $1`, treeID).Scan(&ownerID, &name); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, "", ErrNotFound
		}
		return uuid.Nil, "", err
	}
	return ownerID, name, nil
}

func (s *Service) loadTargetUser(ctx context.Context, q queryer, userID string) (pgtype.UUID, pgtype.Text, error) {
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return pgtype.UUID{}, pgtype.Text{}, ErrValidation
	}
	var id pgtype.UUID
	var email pgtype.Text
	if err := q.QueryRow(ctx, `SELECT id, email FROM users WHERE id = $1 AND status = 'active'`, userUUID).Scan(&id, &email); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, pgtype.Text{}, ErrNotFound
		}
		return pgtype.UUID{}, pgtype.Text{}, err
	}
	return id, email, nil
}

func validateInviteInput(input InviteInput) (InviteInput, error) {
	input.InviteeUserID = strings.TrimSpace(input.InviteeUserID)
	input.InviteeEmail = strings.ToLower(strings.TrimSpace(input.InviteeEmail))
	input.PermissionLevel = strings.TrimSpace(input.PermissionLevel)
	if input.PermissionLevel == "" {
		input.PermissionLevel = "edit"
	}
	if (input.InviteeUserID == "") == (input.InviteeEmail == "") {
		return InviteInput{}, ErrValidation
	}
	if !validPermissionLevel(input.PermissionLevel) {
		return InviteInput{}, ErrValidation
	}
	if input.InviteeUserID != "" {
		if _, err := uuid.Parse(input.InviteeUserID); err != nil {
			return InviteInput{}, ErrValidation
		}
	} else if !validEmail(input.InviteeEmail) {
		return InviteInput{}, ErrValidation
	}
	return input, nil
}

func validPermissionLevel(value string) bool {
	return value == "view" || value == "edit" || value == "review"
}

func validEmail(value string) bool {
	return value != "" && len([]rune(value)) <= 320 && strings.Contains(value, "@") && !strings.ContainsAny(value, " \t\r\n")
}

func parseIDs(treeID, actorID string) (uuid.UUID, uuid.UUID, error) {
	treeUUID, err := uuid.Parse(strings.TrimSpace(treeID))
	if err != nil {
		return uuid.Nil, uuid.Nil, ErrNotFound
	}
	actorUUID, err := uuid.Parse(strings.TrimSpace(actorID))
	if err != nil {
		return uuid.Nil, uuid.Nil, ErrForbidden
	}
	return treeUUID, actorUUID, nil
}

func nullableUUID(value pgtype.UUID) any {
	if !value.Valid {
		return nil
	}
	return uuid.UUID(value.Bytes)
}

func nullableText(value pgtype.Text) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func inviteeUserIDString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func inviteeText(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func writeLifecycle(ctx context.Context, tx pgx.Tx, treeID uuid.UUID, versionID *uuid.UUID, actorID uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any, reason string) error {
	beforeValue := marshalValue(before)
	afterValue := marshalValue(after)
	var versionArg any
	if versionID != nil {
		versionArg = *versionID
	}
	var reasonArg any
	if strings.TrimSpace(reason) != "" {
		reasonArg = strings.TrimSpace(reason)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tree_change_log
			(tree_id, tree_version_id, actor_id, action, entity_type, entity_id, before_value, after_value, reason_ar)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, treeID, versionArg, actorID, action, entityType, entityID, beforeValue, afterValue, reasonArg); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log
			(actor_id, action, entity_type, entity_id, before_value, after_value, reason_ar)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, actorID, action, entityType, entityID, beforeValue, afterValue, reasonArg); err != nil {
		return err
	}
	return nil
}

func marshalValue(value any) any {
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return encoded
}

func scanInvitation(rows pgx.Rows) (InvitationView, error) {
	var item InvitationView
	var id, treeID pgtype.UUID
	var inviteeUserID pgtype.UUID
	var inviteeEmail pgtype.Text
	var acceptedAt pgtype.Timestamptz
	if err := rows.Scan(&id, &treeID, &item.TreeName, &inviteeUserID, &inviteeEmail, &item.PermissionLevel, &item.Status, &item.ExpiresAt, &acceptedAt, &item.CreatedAt); err != nil {
		return InvitationView{}, err
	}
	item.ID = uuidString(id)
	item.TreeID = uuidString(treeID)
	item.InviteeUserID = uuidString(inviteeUserID)
	item.InviteeEmail = textValue(inviteeEmail)
	if acceptedAt.Valid {
		value := acceptedAt.Time
		item.AcceptedAt = &value
	}
	return item, nil
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
