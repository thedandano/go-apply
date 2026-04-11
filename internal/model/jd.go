package model

type SeniorityLevel string

const (
	SeniorityJunior   SeniorityLevel = "junior"
	SeniorityMid      SeniorityLevel = "mid"
	SenioritySenior   SeniorityLevel = "senior"
	SeniorityLead     SeniorityLevel = "lead"
	SeniorityDirector SeniorityLevel = "director"
)

type JDData struct {
	Title         string         `json:"title"`
	Company       string         `json:"company"`
	Required      []string       `json:"required"`
	Preferred     []string       `json:"preferred"`
	Location      string         `json:"location"`
	Seniority     SeniorityLevel `json:"seniority"`
	RequiredYears float64        `json:"required_years"`
	Degraded      bool           `json:"degraded"`
}
