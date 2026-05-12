package bolascope

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer_Pass(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "pass")
}

func TestAnalyzer_Fail(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "fail")
}

func TestAnalyzer_Allow(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "allow")
}
