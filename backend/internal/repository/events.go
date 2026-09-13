package repository

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

func payloadHash(payload any) string {
	b, _ := json.Marshal(payload)
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h[:8])
}

// CheckAndClaimEvent atomically checks if event already processed, and claims it for processing.
// Returns true if already processed (should skip), false if claimed for processing.
func (db *DB) CheckAndClaimEvent(eventID, eventType, source, consumer string, correlationID *uuid.UUID, payload any) (alreadyProcessed bool, err error) {
	hash := payloadHash(payload)
	var corr sql.NullString
	if correlationID != nil {
		corr = sql.NullString{String: correlationID.String(), Valid: true}
	}
	// Try to insert as processing, ON CONFLICT indicates already claimed/processed
	_, err = db.Exec(`INSERT INTO processed_events (event_id, event_type, source, correlation_id, payload_hash, consumer, status) VALUES ($1,$2,$3,$4,$5,$6,'processing') ON CONFLICT (event_id, consumer) DO NOTHING`, eventID, eventType, source, corr, hash, consumer)
	if err != nil {
		return false, err
	}
	var status string
	err = db.QueryRow(`SELECT status FROM processed_events WHERE event_id=$1 AND consumer=$2`, eventID, consumer).Scan(&status)
	if err != nil {
		return false, err
	}
	if status == "processed" {
		// If we just inserted as processing, status is processing, not processed. Check if it was existing processed.
		// The ON CONFLICT DO NOTHING means if existing row was processed, we still have processed status.
		// We need to distinguish: if we inserted, status will be processing; if existing was processed, status remains processed.
		// So if status == "processed" after our insert attempt, it was already processed.
		return true, nil
	}
	// We claimed it as processing
	return false, nil
}

func (db *DB) MarkEventProcessed(eventID, consumer string) error {
	_, err := db.Exec(`UPDATE processed_events SET status='processed', processed_at=now() WHERE event_id=$1 AND consumer=$2`, eventID, consumer)
	return err
}

func (db *DB) MarkEventFailed(eventID, consumer string, errMsg string) error {
	_, err := db.Exec(`UPDATE processed_events SET status='failed', processed_at=now() WHERE event_id=$1 AND consumer=$2`, eventID, consumer)
	return err
}

func (db *DB) IsEventProcessed(eventID, consumer string) (bool, error) {
	var exists bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id=$1 AND consumer=$2 AND status='processed')`, eventID, consumer).Scan(&exists)
	return exists, err
}
