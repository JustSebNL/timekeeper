package api

import (
	"strings"
	"testing"

	"github.com/JustSebNL/timekeeper/internal/routing"
)

func TestRoutingApplyBatchRejectsActionsOutsideInitialScope(t *testing.T) {
	for _, action := range []string{routing.ActionCreateNewCategory, routing.ActionResumeExisting, routing.ActionHoldForReview} {
		t.Run(action, func(t *testing.T) {
			_, err := routingApplyBatch(routingApplyRequest{Decision: routing.Decision{Work: routing.WorkDecision{Disposition: action}}})
			if err == nil || !strings.Contains(err.Error(), "currently supports only create_sprint and create_task_and_sprint") {
				t.Fatalf("routingApplyBatch(%q) error = %v; action must remain out of scope", action, err)
			}
		})
	}
}
