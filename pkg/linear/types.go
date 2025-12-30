// Package linear maintains the logic for interacting with the linear API
package linear

import (
	"encoding/json"
	"time"
)

// LinearResponse structures
type LinearResponse struct {
	Data json.RawMessage `json:"data"`
}

type IssueConnection struct {
	Edges []struct {
		Node Issue `json:"node"`
	} `json:"edges"`
	PageInfo struct {
		HasNextPage bool   `json:"hasNextPage"`
		EndCursor   string `json:"endCursor"`
	} `json:"pageInfo"`
}

type Issue struct {
	ID         string `json:"ID"`
	Identifier string `json:"identifier"`
	Title      string `json:"title"`
	Priority   int    `json:"priority"`
	State      struct {
		Name string `json:"name"`
	} `json:"state"`
	Team struct {
		Name string `json:"name"`
	} `json:"team"`
	Cycle struct {
		Name string `json:"name"`
	} `json:"cycle"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type TeamData struct {
	Teams []Team `json:"teams"`
}

type Team struct {
	Name string `json:"name"`
}
