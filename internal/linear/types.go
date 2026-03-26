// Package linear provides a GraphQL client and types for the Linear API.
package linear

import (
	"encoding/json"
	"time"
)

// GraphQL response wrapper
type GraphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []GraphQLError  `json:"errors,omitempty"`
}

type GraphQLError struct {
	Message string `json:"message"`
}

// Pagination
type PageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// Issue types
type IssueConnection struct {
	Nodes    []Issue  `json:"nodes"`
	PageInfo PageInfo `json:"pageInfo"`
}

type Issue struct {
	ID          string     `json:"id"`
	Identifier  string     `json:"identifier"`
	Title       string     `json:"title"`
	Priority    int        `json:"priority"`
	Estimate    *float64   `json:"estimate"`
	DueDate     *string    `json:"dueDate"`
	StartedAt   *time.Time `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt"`
	CanceledAt  *time.Time `json:"canceledAt"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	State       WorkflowState `json:"state"`
	Team        TeamRef       `json:"team"`
	Assignee    *UserRef      `json:"assignee"`
	Project     *ProjectRef   `json:"project"`
	Cycle       *CycleRef     `json:"cycle"`
	Labels      LabelConnection `json:"labels"`
	Relations   RelationConnection `json:"relations"`
}

type WorkflowState struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // triage, backlog, unstarted, started, completed, canceled
}

type TeamRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"`
}

type UserRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ProjectRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	SlugID string `json:"slugId"`
}

type CycleRef struct {
	ID     string  `json:"id"`
	Name   *string `json:"name"`
	Number int     `json:"number"`
}

type LabelConnection struct {
	Nodes []Label `json:"nodes"`
}

type Label struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RelationConnection struct {
	Nodes []IssueRelation `json:"nodes"`
}

type IssueRelation struct {
	Type string `json:"type"` // blocks, is-blocked-by, related, duplicate
}

// Cycle types
type CycleConnection struct {
	Nodes    []Cycle  `json:"nodes"`
	PageInfo PageInfo `json:"pageInfo"`
}

type Cycle struct {
	ID             string     `json:"id"`
	Name           *string    `json:"name"`
	Number         int        `json:"number"`
	StartsAt       time.Time  `json:"startsAt"`
	EndsAt         time.Time  `json:"endsAt"`
	CompletedAt    *time.Time `json:"completedAt"`
	Progress       float64    `json:"progress"`
	ScopedIssueCount   int   `json:"scopedIssueCount,omitempty"`
	CompletedIssueCount int  `json:"completedIssueCount,omitempty"`
	Team           TeamRef    `json:"team"`
	Issues         IssueConnection `json:"issues"`
}

// Project types
type ProjectConnection struct {
	Nodes    []Project `json:"nodes"`
	PageInfo PageInfo  `json:"pageInfo"`
}

type Project struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	SlugID      string   `json:"slugId"`
	Progress    float64  `json:"progress"`
	State       string   `json:"state"`
	Health      string   `json:"health"` // onTrack, atRisk, offTrack
	Priority    *int     `json:"priority"`
	StartDate   *string  `json:"startDate"`
	TargetDate  *string  `json:"targetDate"`
	Status      *struct {
		Name string `json:"name"`
	} `json:"status"`
	Lead *UserRef `json:"lead"`
	Teams struct {
		Nodes []TeamRef `json:"nodes"`
	} `json:"teams"`
	Milestones struct {
		Nodes []Milestone `json:"nodes"`
	} `json:"projectMilestones"`
	Issues IssueConnection `json:"issues"`
}

// Milestone types
type Milestone struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	TargetDate  *string    `json:"targetDate"`
	SortOrder   float64    `json:"sortOrder"`
}

// Team types
type TeamConnection struct {
	Nodes    []Team   `json:"nodes"`
	PageInfo PageInfo `json:"pageInfo"`
}

type Team struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Key         string `json:"key"`
	Members     struct {
		Nodes []UserRef `json:"nodes"`
	} `json:"members"`
	States struct {
		Nodes []WorkflowState `json:"nodes"`
	} `json:"states"`
}

// User/Member types
type UserConnection struct {
	Nodes    []User   `json:"nodes"`
	PageInfo PageInfo `json:"pageInfo"`
}

type User struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Active      bool   `json:"active"`
	AssignedIssues IssueConnection `json:"assignedIssues"`
}

// WorkflowState connection
type WorkflowStateConnection struct {
	Nodes    []WorkflowState `json:"nodes"`
	PageInfo PageInfo        `json:"pageInfo"`
}

// Label connection for top-level queries
type IssueLabelConnection struct {
	Nodes    []IssueLabel `json:"nodes"`
	PageInfo PageInfo     `json:"pageInfo"`
}

type IssueLabel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Issues IssueConnection `json:"issues"`
}

// Priority helpers
var PriorityNames = map[int]string{
	0: "none",
	1: "urgent",
	2: "high",
	3: "medium",
	4: "low",
}

func PriorityName(p int) string {
	if name, ok := PriorityNames[p]; ok {
		return name
	}
	return "unknown"
}

// Health helpers
func HealthToFloat(health string) float64 {
	switch health {
	case "onTrack":
		return 0
	case "atRisk":
		return 1
	case "offTrack":
		return 2
	default:
		return -1
	}
}
