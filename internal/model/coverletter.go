package model

type ChannelType string

const (
	ChannelCold      ChannelType = "COLD"
	ChannelReferral  ChannelType = "REFERRAL"
	ChannelRecruiter ChannelType = "RECRUITER"
)

type CoverLetterResult struct {
	Text          string      `json:"text"`
	Channel       ChannelType `json:"channel"`
	WordCount     int         `json:"word_count"`
	SentenceCount int         `json:"sentence_count"`
}

// CoverLetterInput groups all inputs required to generate a cover letter.
type CoverLetterInput struct {
	JD        JDData
	JDRawText string // full job description text for richer prompt context
	Scores    map[string]ScoreResult
	Channel   ChannelType
	Profile   UserProfile
}
