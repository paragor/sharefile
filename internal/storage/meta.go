package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
)

const currentVersion = 1
const metadataFile = "metadata.json"

type Metadata struct {
	Version   int    `json:"version"`
	Email     string `json:"email"`
	RssSecret string `json:"rss_secret"`
}

func newMetadata(email string) *Metadata {
	return &Metadata{
		Version:   currentVersion,
		Email:     email,
		RssSecret: uuid.New().String(),
	}
}

func readMetadata(reader io.Reader) (*Metadata, error) {
	obj := &Metadata{}
	if err := json.NewDecoder(reader).Decode(obj); err != nil {
		return nil, fmt.Errorf("cant unmarshal metadata from json: %w", err)
	}
	if obj.Version != currentVersion {
		return nil, fmt.Errorf("object has invalid version: %d", obj.Version)
	}
	if obj.Email == "" {
		return nil, fmt.Errorf("object does not contain email field")
	}
	if obj.RssSecret == "" {
		return nil, fmt.Errorf("object does not contain rss secret field")
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
