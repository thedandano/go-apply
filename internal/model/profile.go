package model

type UserProfile struct {
	Name              string
	Occupation        string
	Location          string
	LinkedInURL       string
	YearsOfExperience float64
	Seniority         string
}

type ProfileEmbedding struct {
	ID        int64
	SourceDoc string
	Term      string
	Weight    float64
}

// ProfileDocument is a stored document chunk without embedding metadata.
// Used by the keyword fallback retrieval path.
type ProfileDocument struct {
	ID     int64
	Source string
	Text   string
}
