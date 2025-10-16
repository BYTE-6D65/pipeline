package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BYTE-6D65/pipeline/cmd/behavior-test/framework"
	"github.com/BYTE-6D65/pipeline/cmd/behavior-test/tests"
)

const suiteName = "Pipeline Dynamic Behavior Test Suite"

func main() {
	// CLI flags
	var (
		runAll     = flag.Bool("all", false, "Run all tests")
		category   = flag.String("category", "", "Run tests in specific category")
		testName   = flag.String("test", "", "Run specific test (e.g., 1.1)")
		verbose    = flag.Bool("verbose", false, "Verbose output (detailed results)")
		reportType = flag.String("report", "summary", "Report type: summary, detailed, json, markdown")
		timeout    = flag.Duration("timeout", 10*time.Minute, "Timeout per test")
	)
	flag.Parse()

	// Validate flags
	if !*runAll && *category == "" && *testName == "" {
		fmt.Println("Error: Must specify --all, --category, or --test")
		flag.Usage()
		os.Exit(1)
	}

	// Build test registry
	testRegistry := buildTestRegistry()

	// Filter tests based on flags
	testsToRun := filterTests(testRegistry, *runAll, *category, *testName)

	if len(testsToRun) == 0 {
		fmt.Println("No tests match the specified criteria")
		os.Exit(1)
	}

	// Create report
	report := framework.NewTestReport(suiteName)

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n\nReceived interrupt signal, stopping tests...")
		cancel()
	}()

	// Run tests
	fmt.Printf("=== %s ===\n\n", suiteName)
	fmt.Printf("Running %d test(s)...\n\n", len(testsToRun))

	for i, test := range testsToRun {
		if ctx.Err() != nil {
			fmt.Println("Tests interrupted by user")
			break
		}

		fmt.Printf("[%d/%d] Running: %s...\n", i+1, len(testsToRun), test.Name())

		result := runTest(ctx, test, *timeout)
		report.AddResult(result)

		// Print result immediately
		if result.Passed {
			fmt.Printf("  ✅ PASS (%s)\n", result.Duration)
		} else {
			fmt.Printf("  ❌ FAIL (%s)\n", result.Duration)
			if !*verbose {
				// Print failed assertions in summary mode
				for _, assertion := range result.Assertions {
					if !assertion.Passed {
						fmt.Printf("    ✗ %s\n", assertion.Name)
						if assertion.Message != "" {
							fmt.Printf("      %s\n", assertion.Message)
						}
					}
				}
			}
		}
		fmt.Println()
	}

	report.Finish()

	// Print report based on type
	fmt.Println()
	switch *reportType {
	case "summary":
		report.PrintSummary(os.Stdout)
	case "detailed":
		report.PrintDetailed(os.Stdout)
	case "json":
		if err := report.PrintJSON(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error printing JSON report: %v\n", err)
			os.Exit(1)
		}
	case "markdown":
		report.PrintMarkdown(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "Unknown report type: %s\n", *reportType)
		os.Exit(1)
	}

	// Exit with appropriate code
	if report.FailedTests() > 0 {
		os.Exit(1)
	}
}

// runTest executes a single test with timeout.
func runTest(ctx context.Context, test framework.TestCase, timeout time.Duration) *framework.TestResult {
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Setup
	if err := test.Setup(testCtx); err != nil {
		result := framework.NewTestResult(test.Name(), test.Category())
		result.AddError(fmt.Errorf("setup failed: %w", err))
		result.Finish()
		return result
	}

	// Ensure teardown runs
	defer func() {
		if err := test.Teardown(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Teardown failed for %s: %v\n", test.Name(), err)
		}
	}()

	// Run
	if err := test.Run(testCtx); err != nil {
		result := framework.NewTestResult(test.Name(), test.Category())
		result.AddError(fmt.Errorf("run failed: %w", err))
		result.Finish()
		return result
	}

	// Validate
	return test.Validate()
}

// buildTestRegistry creates the registry of all available tests.
func buildTestRegistry() []framework.TestCase {
	return []framework.TestCase{
		// Category: Core AIMD Cycle Tests
		tests.NewTest11NormalToDegraded(),
		tests.NewTest12DegradedToRecovering(),
		tests.NewTest13RecoveringToNormal(),

		// Category: Cooldown Enforcement Tests
		tests.NewTest21PanicSawPrevention(),
		// tests.NewTest22RecoveryCooldown(), // TODO: Future

		// Category: Hysteresis and Oscillation Tests
		// tests.NewTest31HysteresisGap(), // TODO: Future
		// tests.NewTest32RecoveryInterruption(), // TODO: Future

		// Category: MinScale Floor Tests
		tests.NewTest41MinScaleFloor(),

		// Category: Full Cycle Stress Tests
		// tests.NewTest51MultipleCycles(), // TODO: Future
		// tests.NewTest52SustainedPressure(), // TODO: Future
	}
}

// filterTests filters the test registry based on CLI flags.
func filterTests(registry []framework.TestCase, all bool, category, testName string) []framework.TestCase {
	if all {
		return registry
	}

	filtered := make([]framework.TestCase, 0)

	for _, test := range registry {
		// Filter by test name
		if testName != "" {
			if test.Name() == testName || contains(test.Name(), testName) {
				filtered = append(filtered, test)
			}
			continue
		}

		// Filter by category
		if category != "" {
			if test.Category() == category || contains(test.Category(), category) {
				filtered = append(filtered, test)
			}
			continue
		}
	}

	return filtered
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}
