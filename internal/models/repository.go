package models

type CommonRepository struct {
	Name     *string `json:"name,omitempty"`
	FullName *string `json:"full_name,omitempty"`
	Owner    *struct {
		Login *string `json:"login,omitempty"`
	} `json:"owner,omitempty"`
	CustomProperties map[string]string `json:"custom_properties,omitempty"`
}

type EventRepository struct {
	Repository CommonRepository `json:"repository"`
}
