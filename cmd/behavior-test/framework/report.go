package framework

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// TestReport contains results from multiple tests.
type TestReport struct {
	SuiteName string
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
	Results   []*TestResult
}

// NewTestReport creates a new test report.
func NewTestReport(suiteName string) *TestReport {
	return &TestReport{
		SuiteName: suiteName,
		StartTime: time.Now(),
		Results:   make([]*TestResult, 0),
	}
}

// AddResult adds a test result to the report.
func (r *TestReport) AddResult(result *TestResult) {
	r.Results = append(r.Results, result)
}

// Finish marks the report as complete.
func (r *TestReport) Finish() {
	r.EndTime = time.Now()
	r.Duration = r.EndTime.Sub(r.StartTime)
}

// TotalTests returns the total number of tests.
func (r *TestReport) TotalTests() int {
	return len(r.Results)
}

// PassedTests returns the number of passed tests.
func (r *TestReport) PassedTests() int {
	count := 0
	for _, result := range r.Results {
		if result.Passed {
			count++
		}
	}
	return count
}

// FailedTests returns the number of failed tests.
func (r *TestReport) FailedTests() int {
	return r.TotalTests() - r.PassedTests()
}

// PassRate returns the percentage of passed tests.
func (r *TestReport) PassRate() float64 {
	if r.TotalTests() == 0 {
		return 0
	}
	return float64(r.PassedTests()) / float64(r.TotalTests()) * 100
}

// PrintSummary prints a text summary to the writer.
func (r *TestReport) PrintSummary(w io.Writer) {
	fmt.Fprintf(w, "=== %s ===\n\n", r.SuiteName)

	// Group by category
	categories := make(map[string][]*TestResult)
	for _, result := range r.Results {
		categories[result.Category] = append(categories[result.Category], result)
	}

	// Print each category
	for category, results := range categories {
		fmt.Fprintf(w, "Category: %s\n", category)
		for _, result := range results {
			status := "PASS"
			symbol := "✅"
			if !result.Passed {
				status = "FAIL"
				symbol = "❌"
			}
			fmt.Fprintf(w, "  [%s] %s %s (%s)\n", status, symbol, result.TestName, result.Duration)
		}
		fmt.Fprintln(w)
	}

	// Print summary
	fmt.Fprintf(w, "=== Summary ===\n")
	fmt.Fprintf(w, "Total Tests: %d\n", r.TotalTests())
	fmt.Fprintf(w, "Passed: %d\n", r.PassedTests())
	fmt.Fprintf(w, "Failed: %d\n", r.FailedTests())
	fmt.Fprintf(w, "Pass Rate: %.1f%%\n", r.PassRate())
	fmt.Fprintf(w, "Duration: %s\n\n", r.Duration)

	if r.FailedTests() == 0 {
		fmt.Fprintf(w, "All tests PASSED ✅\n")
	} else {
		fmt.Fprintf(w, "Some tests FAILED ❌\n")
	}
}

// PrintDetailed prints detailed results to the writer.
func (r *TestReport) PrintDetailed(w io.Writer) {
	fmt.Fprintf(w, "=== %s - Detailed Results ===\n\n", r.SuiteName)

	for _, result := range r.Results {
		r.printTestResult(w, result)
		fmt.Fprintln(w)
	}

	r.PrintSummary(w)
}

func (r *TestReport) printTestResult(w io.Writer, result *TestResult) {
	status := "PASS"
	symbol := "✅"
	if !result.Passed {
		status = "FAIL"
		symbol = "❌"
	}

	fmt.Fprintf(w, "[%s] %s %s\n", status, symbol, result.TestName)
	fmt.Fprintf(w, "Category: %s\n", result.Category)
	fmt.Fprintf(w, "Duration: %s\n", result.Duration)
	fmt.Fprintln(w)

	// Print assertions
	if len(result.Assertions) > 0 {
		fmt.Fprintf(w, "Assertions:\n")
		for i, assertion := range result.Assertions {
			assertSymbol := "✓"
			if !assertion.Passed {
				assertSymbol = "✗"
			}
			fmt.Fprintf(w, "  %d. %s %s\n", i+1, assertSymbol, assertion.Name)
			if !assertion.Passed {
				fmt.Fprintf(w, "     Expected: %v\n", assertion.Expected)
				fmt.Fprintf(w, "     Actual: %v\n", assertion.Actual)
				if assertion.Message != "" {
					fmt.Fprintf(w, "     %s\n", assertion.Message)
				}
			}
		}
		fmt.Fprintln(w)
	}

	// Print metrics
	if len(result.Metrics) > 0 {
		fmt.Fprintf(w, "Metrics:\n")
		for k, v := range result.Metrics {
			fmt.Fprintf(w, "  %s: %v\n", k, v)
		}
		fmt.Fprintln(w)
	}

	// Print errors
	if len(result.Errors) > 0 {
		fmt.Fprintf(w, "Errors:\n")
		for i, err := range result.Errors {
			fmt.Fprintf(w, "  %d. %v\n", i+1, err)
		}
		fmt.Fprintln(w)
	}

	// Print warnings
	if len(result.Warnings) > 0 {
		fmt.Fprintf(w, "Warnings:\n")
		for i, warning := range result.Warnings {
			fmt.Fprintf(w, "  %d. %s\n", i+1, warning)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 80))
}

// PrintJSON prints results as JSON to the writer.
func (r *TestReport) PrintJSON(w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(r)
}

// PrintMarkdown prints results as Markdown to the writer.
func (r *TestReport) PrintMarkdown(w io.Writer) {
	fmt.Fprintf(w, "# %s\n\n", r.SuiteName)
	fmt.Fprintf(w, "**Date**: %s\n", r.StartTime.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "**Duration**: %s\n\n", r.Duration)

	// Summary table
	fmt.Fprintf(w, "## Summary\n\n")
	fmt.Fprintf(w, "| Metric | Value |\n")
	fmt.Fprintf(w, "|--------|-------|\n")
	fmt.Fprintf(w, "| Total Tests | %d |\n", r.TotalTests())
	fmt.Fprintf(w, "| Passed | %d |\n", r.PassedTests())
	fmt.Fprintf(w, "| Failed | %d |\n", r.FailedTests())
	fmt.Fprintf(w, "| Pass Rate | %.1f%% |\n\n", r.PassRate())

	// Group by category
	categories := make(map[string][]*TestResult)
	for _, result := range r.Results {
		categories[result.Category] = append(categories[result.Category], result)
	}

	// Print each category
	for category, results := range categories {
		fmt.Fprintf(w, "## %s\n\n", category)
		fmt.Fprintf(w, "| Test | Status | Duration |\n")
		fmt.Fprintf(w, "|------|--------|----------|\n")
		for _, result := range results {
			status := "✅ PASS"
			if !result.Passed {
				status = "❌ FAIL"
			}
			fmt.Fprintf(w, "| %s | %s | %s |\n", result.TestName, status, result.Duration)
		}
		fmt.Fprintln(w)
	}

	// Overall status
	if r.FailedTests() == 0 {
		fmt.Fprintf(w, "## Result\n\n✅ **All tests PASSED**\n")
	} else {
		fmt.Fprintf(w, "## Result\n\n❌ **%d test(s) FAILED**\n", r.FailedTests())
	}
}
