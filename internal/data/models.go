package data

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"github.com/lib/pq"
	null "gopkg.in/volatiletech/null.v6"
)

// JSON is is the wrapper for reading and writing arbitrary JSONB fields from the DB.
type JSON map[string]interface{}

// Entry represents a dictionary entry.
type Entry struct {
	ID        int            `json:"id,omitempty" db:"id"`
	GUID      string         `json:"guid" db:"guid"`
	Weight    float64        `json:"weight" db:"weight"`
	Initial   string         `json:"initial" db:"initial"`
	Lang      string         `json:"lang" db:"lang"`
	Content   string         `json:"content" db:"content"`
	Tokens    string         `json:"tokens" db:"tokens"`
	Tags      pq.StringArray `json:"tags" db:"tags"`
	Phones    pq.StringArray `json:"phones" db:"phones"`
	Notes     string         `json:"notes" db:"notes"`
	Meta      JSON           `json:"meta" db:"meta"`
	Status    string         `json:"status" db:"status"`
	Relations []Entry        `json:"relations,omitempty" db:"relations"`
	Total     int            `json:"-" db:"total"`
	CreatedAt null.Time      `json:"created_at" db:"created_at"`
	UpdatedAt null.Time      `json:"updated_at" db:"updated_at"`

	// Non-public fields for scanning relationship data and populating Relation.
	FromID            int            `json:"-" db:"from_id"`
	RelationID        int            `json:"-" db:"relation_id"`
	RelationTypes     pq.StringArray `json:"-" db:"relation_types"`
	RelationTags      pq.StringArray `json:"-" db:"relation_tags"`
	RelationNotes     string         `json:"-" db:"relation_notes"`
	RelationWeight    float64        `json:"-" db:"relation_weight"`
	RelationStatus    string         `json:"-" db:"relation_status"`
	RelationCreatedAt null.Time      `json:"-" db:"relation_created_at"`
	RelationUpdatedAt null.Time      `json:"-" db:"relation_updated_at"`

	// RelationEntry encompasses an Entry with added fields that
	// describes its relationship to other []Entry. This is only populated in
	// []Entry in the Relations list.
	Relation *Relation `json:"relation,omitempty"`
}

// Relation represents the relationship between two IDs.
type Relation struct {
	ID        int            `json:"id,omitempty"`
	Types     pq.StringArray `json:"types"`
	Tags      pq.StringArray `json:"tags"`
	Notes     string         `json:"notes"`
	Weight    float64        `json:"weight"`
	Status    string         `json:"status"`
	CreatedAt null.Time      `json:"created_at"`
	UpdatedAt null.Time      `json:"updated_at"`
}

// GlossaryWord to read glosary content from db.
type GlossaryWord struct {
	ID      int    `json:"id,omitempty" db:"id"`
	Content string `json:"content" db:"content"`
	Total   int    `json:"-" db:"total"`
}

// Stats contains database statistics.
type Stats struct {
	Entries   int            `json:"entries"`
	Relations int            `json:"relations"`
	Languages map[string]int `json:"languages"`
}

type Comments struct {
	ID       int      `json:"id" db:"id"`
	FromID   int      `json:"from_id" db:"from_id"`
	ToID     null.Int `json:"to_id" db:"to_id"`
	Comments string   `json:"comments" db:"comments"`
}

// Value returns the JSON marshalled SubscriberAttribs.
func (s JSON) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// Scan unmarshals JSONB from the DB.
func (s JSON) Scan(src interface{}) error {
	if src == nil {
		s = make(JSON)
		return nil
	}

	if data, ok := src.([]byte); ok {
		return json.Unmarshal(data, &s)
	}
	return fmt.Errorf("could not not decode type %T -> %T", src, s)
}
