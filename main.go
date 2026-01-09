package main

import (
	"context"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"sync"

	l "github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

var (
	logger *slog.Logger
	dryRun bool
)

const (
	versionsToKeep = 5
)

func init() {
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	dryRun = os.Getenv("DRY_RUN") == "true"
	if dryRun {
		logger.Info("Running in DRY RUN mode - no deletions will be performed")
	}
}

// LambdaAPI defines the interface for Lambda operations
type LambdaAPI interface {
	ListFunctions(ctx context.Context, input *lambda.ListFunctionsInput, opts ...func(*lambda.Options)) (*lambda.ListFunctionsOutput, error)
	ListVersionsByFunction(ctx context.Context, input *lambda.ListVersionsByFunctionInput, opts ...func(*lambda.Options)) (*lambda.ListVersionsByFunctionOutput, error)
	ListAliases(ctx context.Context, input *lambda.ListAliasesInput, opts ...func(*lambda.Options)) (*lambda.ListAliasesOutput, error)
	DeleteFunction(ctx context.Context, input *lambda.DeleteFunctionInput, opts ...func(*lambda.Options)) (*lambda.DeleteFunctionOutput, error)
}

type LambdaJanitor struct {
	client LambdaAPI
}

func NewLambdaJanitor(ctx context.Context) (*LambdaJanitor, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	return &LambdaJanitor{
		client: lambda.NewFromConfig(cfg),
	}, nil
}

// listAllFunctions handles pagination to get all Lambda functions
func (j *LambdaJanitor) listAllFunctions(ctx context.Context) ([]lambdaTypes.FunctionConfiguration, error) {
	var functions []lambdaTypes.FunctionConfiguration
	var nextMarker *string

	for {
		input := &lambda.ListFunctionsInput{
			Marker: nextMarker,
		}

		output, err := j.client.ListFunctions(ctx, input)
		if err != nil {
			return nil, err
		}

		functions = append(functions, output.Functions...)

		if output.NextMarker == nil {
			break
		}
		nextMarker = output.NextMarker
	}

	logger.Info("Listed all functions", "count", len(functions))
	return functions, nil
}

// listAllVersions handles pagination to get all versions of a function
func (j *LambdaJanitor) listAllVersions(ctx context.Context, functionName string) ([]lambdaTypes.FunctionConfiguration, error) {
	var versions []lambdaTypes.FunctionConfiguration
	var nextMarker *string

	for {
		input := &lambda.ListVersionsByFunctionInput{
			FunctionName: aws.String(functionName),
			Marker:       nextMarker,
		}

		output, err := j.client.ListVersionsByFunction(ctx, input)
		if err != nil {
			return nil, err
		}

		versions = append(versions, output.Versions...)

		if output.NextMarker == nil {
			break
		}
		nextMarker = output.NextMarker
	}

	return versions, nil
}

// getAliasedVersions returns a set of version numbers that have aliases
func (j *LambdaJanitor) getAliasedVersions(ctx context.Context, functionName string) (map[string]bool, error) {
	aliasedVersions := make(map[string]bool)
	var nextMarker *string

	for {
		input := &lambda.ListAliasesInput{
			FunctionName: aws.String(functionName),
			Marker:       nextMarker,
		}

		output, err := j.client.ListAliases(ctx, input)
		if err != nil {
			return nil, err
		}

		for _, alias := range output.Aliases {
			if alias.FunctionVersion != nil {
				aliasedVersions[*alias.FunctionVersion] = true
				logger.Info("Found aliased version",
					"function", functionName,
					"alias", *alias.Name,
					"version", *alias.FunctionVersion)
			}
		}

		if output.NextMarker == nil {
			break
		}
		nextMarker = output.NextMarker
	}

	return aliasedVersions, nil
}

// cleanupFunction processes a single function and deletes old versions
// Returns the number of versions deleted
func (j *LambdaJanitor) cleanupFunction(ctx context.Context, functionName string) (int, error) {
	logger.Info("Processing function", "function", functionName)

	// Get all versions
	versions, err := j.listAllVersions(ctx, functionName)
	if err != nil {
		logger.Error("Failed to list versions", "function", functionName, "error", err)
		return 0, err
	}

	// Get aliased versions
	aliasedVersions, err := j.getAliasedVersions(ctx, functionName)
	if err != nil {
		logger.Error("Failed to list aliases", "function", functionName, "error", err)
		return 0, err
	}

	// Filter out $LATEST and sort versions by number (descending)
	var numericVersions []int
	versionMap := make(map[int]lambdaTypes.FunctionConfiguration)

	for _, version := range versions {
		if version.Version != nil && *version.Version != "$LATEST" {
			versionNum, err := strconv.Atoi(*version.Version)
			if err != nil {
				continue
			}
			numericVersions = append(numericVersions, versionNum)
			versionMap[versionNum] = version
		}
	}

	// Sort in descending order (newest first)
	sort.Sort(sort.Reverse(sort.IntSlice(numericVersions)))

	logger.Info("Found versions",
		"function", functionName,
		"total_versions", len(numericVersions),
		"aliased_count", len(aliasedVersions))

	// Process versions for deletion
	deletedCount := 0
	skippedCount := 0

	for i, versionNum := range numericVersions {
		versionStr := strconv.Itoa(versionNum)

		// Skip if it's in the top N most recent versions
		if i < versionsToKeep {
			logger.Info("Keeping recent version",
				"function", functionName,
				"version", versionStr,
				"reason", "within_retention_period")
			skippedCount++
			continue
		}

		// Skip if version is aliased
		if aliasedVersions[versionStr] {
			logger.Info("Keeping aliased version",
				"function", functionName,
				"version", versionStr,
				"reason", "has_alias")
			skippedCount++
			continue
		}

		// Delete the version
		if dryRun {
			logger.Info("DRY RUN: Would delete version",
				"function", functionName,
				"version", versionStr,
				"status", "dry_run")
			deletedCount++
		} else {
			err := j.deleteVersion(ctx, functionName, versionStr)
			if err != nil {
				logger.Error("Failed to delete version",
					"function", functionName,
					"version", versionStr,
					"error", err)
			} else {
				logger.Info("Successfully deleted version",
					"function", functionName,
					"version", versionStr,
					"status", "deleted")
				deletedCount++
			}
		}
	}

	logger.Info("Cleanup completed for function",
		"function", functionName,
		"deleted", deletedCount,
		"skipped", skippedCount)

	return deletedCount, nil
}

// deleteVersion deletes a specific version of a Lambda function
func (j *LambdaJanitor) deleteVersion(ctx context.Context, functionName, version string) error {
	input := &lambda.DeleteFunctionInput{
		FunctionName: aws.String(functionName),
		Qualifier:    aws.String(version),
	}

	_, err := j.client.DeleteFunction(ctx, input)
	return err
}

// Run executes the janitor cleanup process
func (j *LambdaJanitor) Run(ctx context.Context) error {
	// List all functions
	functions, err := j.listAllFunctions(ctx)
	if err != nil {
		logger.Error("Failed to list functions", "error", err)
		return err
	}

	// Use WaitGroup for concurrent processing
	var wg sync.WaitGroup
	var mu sync.Mutex
	semaphore := make(chan struct{}, 10) // Limit concurrent goroutines to 10
	totalDeletedVersions := 0
	functionsProcessed := 0

	for _, function := range functions {
		if function.FunctionName == nil {
			continue
		}

		wg.Add(1)
		go func(functionName string) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			deletedCount, err := j.cleanupFunction(ctx, functionName)
			if err != nil {
				logger.Error("Error cleaning function", "function", functionName, "error", err)
			} else {
				// Update counters in a thread-safe manner
				mu.Lock()
				totalDeletedVersions += deletedCount
				functionsProcessed++
				mu.Unlock()
			}
		}(*function.FunctionName)
	}

	wg.Wait()

	logger.Info("Janitor cleanup completed",
		"total_functions", len(functions),
		"functions_processed", functionsProcessed,
		"total_versions_deleted", totalDeletedVersions)

	return nil
}

func handler(ctx context.Context) error {
	logger.Info("Starting Lambda Janitor")

	janitor, err := NewLambdaJanitor(ctx)
	if err != nil {
		logger.Error("Failed to initialize janitor", "error", err)
		return err
	}

	return janitor.Run(ctx)
}

func main() {
	// Check if running locally (not in Lambda environment)
	if os.Getenv("LOCAL_MODE") == "true" || os.Getenv("AWS_LAMBDA_RUNTIME_API") == "" {
		logger.Info("Running in local mode")
		ctx := context.Background()
		if err := handler(ctx); err != nil {
			logger.Error("Handler failed", "error", err)
			os.Exit(1)
		}
		logger.Info("Local execution completed successfully")
		return
	}

	// Running in Lambda environment
	l.Start(handler)
}
