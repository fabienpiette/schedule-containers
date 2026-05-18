package models

type CronPreset struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Expression  string `json:"expression"`
	Category    string `json:"category"`
	Description string `json:"description"`
}