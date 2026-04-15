//go:build bdd

package bdd_test

import (
	"testing"

	"github.com/cucumber/godog"

	"github.com/thedandano/go-apply/tests/bdd/steps"
)

func TestBDD(t *testing.T) {
	suite := godog.TestSuite{
		Name: "go-apply BDD",
		ScenarioInitializer: func(ctx *godog.ScenarioContext) {
			steps.InitializeOnboardingScenario(ctx)
			steps.InitializeWorkflowScenario(ctx)
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"./features"},
			Tags:     "~@future",
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("BDD suite failed")
	}
}
