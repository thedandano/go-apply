package model

import (
	"testing"
)

func TestParseChannel(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    ChannelType
		wantErr bool
		errMsg  string
	}{
		{
			name:    "COLD uppercase",
			input:   "COLD",
			want:    ChannelCold,
			wantErr: false,
		},
		{
			name:    "cold lowercase",
			input:   "cold",
			want:    ChannelCold,
			wantErr: false,
		},
		{
			name:    "Cold mixed case",
			input:   "Cold",
			want:    ChannelCold,
			wantErr: false,
		},
		{
			name:    "REFERRAL uppercase",
			input:   "REFERRAL",
			want:    ChannelReferral,
			wantErr: false,
		},
		{
			name:    "referral lowercase",
			input:   "referral",
			want:    ChannelReferral,
			wantErr: false,
		},
		{
			name:    "RECRUITER uppercase",
			input:   "RECRUITER",
			want:    ChannelRecruiter,
			wantErr: false,
		},
		{
			name:    "recruiter lowercase",
			input:   "recruiter",
			want:    ChannelRecruiter,
			wantErr: false,
		},
		{
			name:    "unknown channel",
			input:   "UNKNOWN",
			want:    "",
			wantErr: true,
			errMsg:  "unknown channel",
		},
		{
			name:    "empty string",
			input:   "",
			want:    "",
			wantErr: true,
			errMsg:  "unknown channel",
		},
		{
			name:    "invalid value",
			input:   "INVALID",
			want:    "",
			wantErr: true,
			errMsg:  "unknown channel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseChannel(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("ParseChannel(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("ParseChannel(%q) error message = %q, want to contain %q", tt.input, err.Error(), tt.errMsg)
				}
				return
			}

			if got != tt.want {
				t.Errorf("ParseChannel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
