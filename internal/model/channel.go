package model

import (
	"fmt"
	"strings"
)

// ParseChannel parses a channel string into a ChannelType.
func ParseChannel(s string) (ChannelType, error) {
	switch strings.ToUpper(s) {
	case "COLD":
		return ChannelCold, nil
	case "REFERRAL":
		return ChannelReferral, nil
	case "RECRUITER":
		return ChannelRecruiter, nil
	default:
		return "", fmt.Errorf("unknown channel %q — valid values: COLD, REFERRAL, RECRUITER", s)
	}
}
