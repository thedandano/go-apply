package model_test

import (
	"testing"

	"github.com/thedandano/go-apply/internal/model"
)

func TestExperienceEntry_BulletID(t *testing.T) {
	e := model.ExperienceEntry{}
	tests := []struct {
		entryIdx  int
		bulletIdx int
		want      string
	}{
		{0, 0, "exp-0-b0"},
		{0, 2, "exp-0-b2"},
		{1, 0, "exp-1-b0"},
		{3, 5, "exp-3-b5"},
	}
	for _, tc := range tests {
		got := e.BulletID(tc.entryIdx, tc.bulletIdx)
		if got != tc.want {
			t.Errorf("BulletID(%d,%d) = %q, want %q", tc.entryIdx, tc.bulletIdx, got, tc.want)
		}
	}
}

func TestExperienceEntry_ID(t *testing.T) {
	e := model.ExperienceEntry{}
	tests := []struct {
		idx  int
		want string
	}{
		{0, "exp-0"},
		{1, "exp-1"},
		{5, "exp-5"},
	}
	for _, tc := range tests {
		got := e.ID(tc.idx)
		if got != tc.want {
			t.Errorf("ID(%d) = %q, want %q", tc.idx, got, tc.want)
		}
	}
}
