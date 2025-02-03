package models

import "time"

// Event represents an AWS EventBridge event.
type Event struct {
	ID         string    `json:"id"`
	Time       time.Time `json:"time"`
	Region     string    `json:"region"`
	Source     string    `json:"source"`
	Account    string    `json:"account"`
	Version    string    `json:"version"`
	Detail     any       `json:"detail"`
	DetailType string    `json:"detail-type"`
	Resources  []string  `json:"resources"`
}
