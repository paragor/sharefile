package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
)

const currentVersion = 2
const metadataFile = "metadata.json"

type Metadata struct {
	Version int    `json:"version"`
	Email   string `json:"email"`
	Secret  string `json:"secret"`

	// removed since v2
	RssSecret string `json:"rss_secret,omitempty"`
}

func (m *Metadata) MigrationRequired() bool {
	return m.Version < currentVersion
}
func (m *Metadata) Migrate() {
	if m.Version == currentVersion {
		return
	}
	if m.Version == 1 {
		m.migrateFromV1()
	}
}
func (m *Metadata) migrateFromV1() {
	m.Version = 2
	m.Secret = m.RssSecret
	m.RssSecret = ""
}

func newMetadata(email string) *Metadata {
	return &Metadata{
		Version: currentVersion,
		Email:   email,
		Secret:  uuid.New().String(),
	}
}

func readMetadata(reader io.Reader) (*Metadata, error) {
	obj := &Metadata{}
	if err := json.NewDecoder(reader).Decode(obj); err != nil {
		return nil, fmt.Errorf("cant unmarshal metadata from json: %w", err)
	}
	if obj.Version > currentVersion || obj.Version < 1 {
		return nil, fmt.Errorf("object has invalid version: %d", obj.Version)
	}
	if obj.Email == "" {
		return nil, fmt.Errorf("object does not contain email field")
	}
	if obj.Version == 1 && obj.RssSecret == "" {
		return nil, fmt.Errorf("object does not contain rss secret field")
	}
	if obj.Version >= 2 && obj.Secret == "" {
		return nil, fmt.Errorf("object does not contain secret field")
	}
	return obj, nil
}

func (m *Metadata) marshal() ([]byte, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("cant marshal metadata into json: %w", err)
	}
	return data, nil
}

type FileInList struct {
	Path           string
	LastModifiedAt time.Time
	Size           int
}
