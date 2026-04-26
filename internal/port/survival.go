package port

import "github.com/thedandano/go-apply/internal/model"

// SurvivalDiffer classifies JD keywords as matched or dropped after the
// PDF render→extract pipeline. Assumes the keyword list is already deduplicated.
type SurvivalDiffer interface {
	Diff(keywords []string, extractedText string) model.KeywordSurvival
}
