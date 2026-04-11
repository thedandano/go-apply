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
	Degraded      bool        `json:"degraded"`
}
