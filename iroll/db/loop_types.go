package db

import (
	"bytes"
	"encoding/json"
)

type LoopSeed struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Describe   string  `json:"describe"`
	Content    string  `json:"content"`
	Weight     float64 `json:"weight"`
	ArchivedAt *string `json:"archived_at"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

type LoopSeedPatch struct {
	Describe *string
	Content  *string
	Weight   *float64
}

type LoopRun struct {
	ID           int64           `json:"run_id"`
	LoopID       int64           `json:"loop_id"`
	PageID       string          `json:"page_id"`
	ParentRunID  *int64          `json:"parent_run_id"`
	SeedName     string          `json:"seed_name"`
	SeedDescribe string          `json:"seed_describe"`
	SeedContent  string          `json:"seed_content"`
	SeedWeight   float64         `json:"seed_weight"`
	Status       string          `json:"status"`
	Plan         json.RawMessage `json:"plan"`
	Progress     json.RawMessage `json:"progress"`
	Result       json.RawMessage `json:"result"`
	Reflection   json.RawMessage `json:"reflection"`
	AbortReason  *string         `json:"abort_reason"`
	StartedAt    string          `json:"started_at"`
	EndedAt      *string         `json:"ended_at"`
	ReflectedAt  *string         `json:"reflected_at"`
	UpdatedAt    string          `json:"updated_at"`
}

func NormalizeLoopJSON(input string) (string, error) {
	if json.Valid([]byte(input)) {
		var compact bytes.Buffer
		if err := json.Compact(&compact, []byte(input)); err != nil {
			return "", err
		}
		return compact.String(), nil
	}
	data, err := json.Marshal(input)
	return string(data), err
}
