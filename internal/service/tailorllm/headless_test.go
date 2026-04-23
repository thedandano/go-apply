package tailorllm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/service/tailorllm"
)

func TestHeadlessNullTailor_ReturnsErrHeadlessTailorNotImplemented(t *testing.T) {
	h := &tailorllm.HeadlessNullTailor{}
	_, err := h.TailorResume(context.Background(), &model.TailorInput{ResumeText: "test"})
	if !errors.Is(err, tailorllm.ErrHeadlessTailorNotImplemented) {
		t.Errorf("TailorResume returned %v, want ErrHeadlessTailorNotImplemented", err)
	}
}
