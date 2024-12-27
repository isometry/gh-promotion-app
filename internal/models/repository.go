package models

// RepositoryContext represents the context of a repository, including its name, full name, owner, and custom properties.
type RepositoryContext struct {
	Name     *string `json:"name,omitempty"`
	FullName *string `json:"full_name,omitempty"`
	Owner    *struct {
		Login *string `json:"login,omitempty"`
	} `json:"owner,omitempty"`
	CustomProperties map[string]string `json:"custom_properties,omitempty"`
}

// EventRepository represents an event's repository details as part of webhook payloads or events.
type EventRepository struct {
	Repository RepositoryContext `json:"repository"`
}
