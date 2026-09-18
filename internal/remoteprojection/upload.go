package remoteprojection

import (
	"encoding/json"
	"time"
)

// ValidateUpload admits the closed connected-upload schema set. Each schema
// retains its own canonical validation; this is not a general JSON validator.
func ValidateUpload(data []byte, expectedServerID string) error {
	_, err := UploadCollectionTime(data, expectedServerID)
	return err
}

// UploadCollectionTime extracts freshness only after complete schema and binding
// validation. Callers must retain the original bytes for sequence-safe retries.
func UploadCollectionTime(data []byte, expectedServerID string) (time.Time, error) {
	if len(data) == 0 || len(data) > MaxBytes {
		return time.Time{}, ErrValidation
	}
	var envelope struct {
		SchemaVersion string `json:"schema_version"`
		CollectedAt   string `json:"collected_at"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return time.Time{}, ErrValidation
	}
	var err error
	switch envelope.SchemaVersion {
	case "home-lab-server-snapshot/v1":
		err = Validate(data, expectedServerID)
	case NativeSchemaVersion:
		err = ValidateNative(data, expectedServerID)
	default:
		err = ErrValidation
	}
	if err != nil {
		return time.Time{}, ErrValidation
	}
	at, err := time.Parse(time.RFC3339Nano, envelope.CollectedAt)
	if err != nil {
		return time.Time{}, ErrValidation
	}
	return at, nil
}
