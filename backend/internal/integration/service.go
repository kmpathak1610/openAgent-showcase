package integration

import (
	"database/sql"
	"encoding/json"

	"github.com/google/uuid"
	"openagent/internal/domain"
	"openagent/internal/repository"
)

type Service struct {
	repo      *repository.DB
	jwtSecret string
	registry  *Registry
}

func NewService(repo *repository.DB, jwtSecret string, reg *Registry) *Service {
	return &Service{repo: repo, jwtSecret: jwtSecret, registry: reg}
}

func (s *Service) Create(orgID uuid.UUID, name, provider string, config, credentials map[string]any, createdBy uuid.UUID) (*domain.Integration, error) {
	// encrypt credentials
	var enc *string
	if len(credentials) > 0 {
		encStr, err := Encrypt(credentials, s.jwtSecret)
		if err != nil { return nil, err }
		enc = &encStr
	}
	configJSON, _ := json.Marshal(config)
	// store placeholder in credentials JSONB (redacted) and encrypted in credentials_encrypted
	credsJSON, _ := json.Marshal(Redacted(credentials))
	var integ domain.Integration
	var cfg, creds []byte
	var credEnc sql.NullString
	var lastSynced sql.NullTime
	err := s.repo.QueryRow(
		`INSERT INTO integrations (organization_id, name, provider, config, credentials, credentials_encrypted, created_by) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, organization_id, name, provider, config, credentials, credentials_encrypted, status, created_by, created_at, updated_at, last_synced_at`,
		orgID, name, provider, configJSON, credsJSON, enc, createdBy,
	).Scan(&integ.ID, &integ.OrganizationID, &integ.Name, &integ.Provider, &cfg, &creds, &credEnc, &integ.Status, &integ.CreatedBy, &integ.CreatedAt, &integ.UpdatedAt, &lastSynced)
	if err != nil { return nil, err }
	if len(cfg)>0 { _ = json.Unmarshal(cfg, &integ.Config) }
	if credEnc.Valid {
		if dec, err := Decrypt(credEnc.String, s.jwtSecret); err == nil {
			integ.Credentials = dec
		}
	}
	integ.CredentialsEncrypted = nil
	if lastSynced.Valid { integ.LastSyncedAt = &lastSynced.Time }
	return &integ, nil
}

func (s *Service) List(orgID uuid.UUID) ([]*domain.Integration, error) {
	rows, err := s.repo.Query(`SELECT id, organization_id, name, provider, config, credentials, credentials_encrypted, status, created_by, created_at, updated_at, last_synced_at FROM integrations WHERE organization_id=$1 ORDER BY created_at`, orgID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*domain.Integration
	for rows.Next() {
		var integ domain.Integration
		var cfg, creds []byte
		var credEnc sql.NullString
		var lastSynced sql.NullTime
		if err := rows.Scan(&integ.ID, &integ.OrganizationID, &integ.Name, &integ.Provider, &cfg, &creds, &credEnc, &integ.Status, &integ.CreatedBy, &integ.CreatedAt, &integ.UpdatedAt, &lastSynced); err != nil { return nil, err }
		if len(cfg)>0 { _ = json.Unmarshal(cfg, &integ.Config) }
		// never return raw encrypted; decrypt for internal use but redact for response
		if credEnc.Valid {
			if dec, err := Decrypt(credEnc.String, s.jwtSecret); err == nil {
				integ.Credentials = Redacted(dec)
			}
		} else if len(creds)>0 {
			_ = json.Unmarshal(creds, &integ.Credentials)
			integ.Credentials = Redacted(integ.Credentials)
		}
		if lastSynced.Valid { integ.LastSyncedAt = &lastSynced.Time }
		out = append(out, &integ)
	}
	return out, nil
}

func (s *Service) Get(orgID, id uuid.UUID) (*domain.Integration, error) {
	var integ domain.Integration
	var cfg, creds []byte
	var credEnc sql.NullString
	var lastSynced sql.NullTime
	err := s.repo.QueryRow(`SELECT id, organization_id, name, provider, config, credentials, credentials_encrypted, status, created_by, created_at, updated_at, last_synced_at FROM integrations WHERE id=$1 AND organization_id=$2`, id, orgID).Scan(
		&integ.ID, &integ.OrganizationID, &integ.Name, &integ.Provider, &cfg, &creds, &credEnc, &integ.Status, &integ.CreatedBy, &integ.CreatedAt, &integ.UpdatedAt, &lastSynced)
	if err != nil { return nil, err }
	if len(cfg)>0 { _ = json.Unmarshal(cfg, &integ.Config) }
	if credEnc.Valid {
		if dec, err := Decrypt(credEnc.String, s.jwtSecret); err == nil {
			integ.Credentials = Redacted(dec)
		}
	}
	if lastSynced.Valid { integ.LastSyncedAt = &lastSynced.Time }
	return &integ, nil
}

func (s *Service) GetDecrypted(orgID, id uuid.UUID) (map[string]any, error) {
	var credEnc sql.NullString
	err := s.repo.QueryRow(`SELECT credentials_encrypted FROM integrations WHERE id=$1 AND organization_id=$2`, id, orgID).Scan(&credEnc)
	if err != nil { return nil, err }
	if !credEnc.Valid || credEnc.String == "" { return map[string]any{}, nil }
	return Decrypt(credEnc.String, s.jwtSecret)
}

func (s *Service) UpdateStatus(orgID, id uuid.UUID, status string) error {
	_, err := s.repo.Exec(`UPDATE integrations SET status=$3, updated_at=now() WHERE id=$1 AND organization_id=$2`, id, orgID, status)
	return err
}

func (s *Service) Delete(orgID, id uuid.UUID) error {
	_, err := s.repo.Exec(`DELETE FROM integrations WHERE id=$1 AND organization_id=$2`, id, orgID)
	return err
}
