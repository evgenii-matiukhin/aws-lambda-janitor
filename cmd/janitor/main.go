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

var logger *slog.Logger

const versionsToKeep = 3 // CDK requires keeping recent published versions

func init() {
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

type LambdaAPI interface {
	ListFunctions(ctx context.Context, input *lambda.ListFunctionsInput, opts ...func(*lambda.Options)) (*lambda.ListFunctionsOutput, error)
	ListVersionsByFunction(ctx context.Context, input *lambda.ListVersionsByFunctionInput, opts ...func(*lambda.Options)) (*lambda.ListVersionsByFunctionOutput, error)
	ListAliases(ctx context.Context, input *lambda.ListAliasesInput, opts ...func(*lambda.Options)) (*lambda.ListAliasesOutput, error)
	DeleteFunction(ctx context.Context, input *lambda.DeleteFunctionInput, opts ...func(*lambda.Options)) (*lambda.DeleteFunctionOutput, error)
}

type LambdaJanitor struct {
	client LambdaAPI
	dryRun bool
}

func NewLambdaJanitor(ctx context.Context) (*LambdaJanitor, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	dryRun := os.Getenv("DRY_RUN") == "true"
	if dryRun {
		logger.Info("Running in DRY RUN mode - no deletions will be performed")
	}

	return &LambdaJanitor{
		client: lambda.NewFromConfig(cfg),
		dryRun: dryRun,
	}, nil
}

func (j *LambdaJanitor) listAllFunctions(ctx context.Context) ([]lambdaTypes.FunctionConfiguration, error) {
	var functions []lambdaTypes.FunctionConfiguration
	var nextMarker *string

	for {
		output, err := j.client.ListFunctions(ctx, &lambda.ListFunctionsInput{Marker: nextMarker})
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

func (j *LambdaJanitor) listAllVersions(ctx context.Context, functionName string) ([]lambdaTypes.FunctionConfiguration, error) {
	var versions []lambdaTypes.FunctionConfiguration
	var nextMarker *string

	for {
		output, err := j.client.ListVersionsByFunction(ctx, &lambda.ListVersionsByFunctionInput{
			FunctionName: aws.String(functionName),
			Marker:       nextMarker,
		})
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

func (j *LambdaJanitor) getAliasedVersions(ctx context.Context, functionName string) (map[string]bool, error) {
	aliasedVersions := make(map[string]bool)
	var nextMarker *string

	for {
		output, err := j.client.ListAliases(ctx, &lambda.ListAliasesInput{
			FunctionName: aws.String(functionName),
			Marker:       nextMarker,
		})
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

func (j *LambdaJanitor) cleanupFunction(ctx context.Context, functionName string) (int, error) {
	logger.Info("Processing function", "function", functionName)

	versions, err := j.listAllVersions(ctx, functionName)
	if err != nil {
		logger.Error("Failed to list versions", "function", functionName, "error", err)
		return 0, err
	}

	aliasedVersions, err := j.getAliasedVersions(ctx, functionName)
	if err != nil {
		logger.Error("Failed to list aliases", "function", functionName, "error", err)
		return 0, err
	}

	var numericVersions []int
	for _, version := range versions {
		if version.Version != nil && *version.Version != "$LATEST" {
			versionNum, err := strconv.Atoi(*version.Version)
			if err != nil {
				continue
			}
			numericVersions = append(numericVersions, versionNum)
		}
	}

	sort.Sort(sort.Reverse(sort.IntSlice(numericVersions)))

	logger.Info("Found versions",
		"function", functionName,
		"total_versions", len(numericVersions),
		"aliased_count", len(aliasedVersions))

	deletedCount := 0
	skippedCount := 0

	for i, versionNum := range numericVersions {
		versionStr := strconv.Itoa(versionNum)

		if i < versionsToKeep {
			skippedCount++
			continue
		}

		if aliasedVersions[versionStr] {
			skippedCount++
			continue
		}

		if j.dryRun {
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

func (j *LambdaJanitor) deleteVersion(ctx context.Context, functionName, version string) error {
	_, err := j.client.DeleteFunction(ctx, &lambda.DeleteFunctionInput{
		FunctionName: aws.String(functionName),
		Qualifier:    aws.String(version),
	})
	return err
}

func (j *LambdaJanitor) Run(ctx context.Context) error {
	functions, err := j.listAllFunctions(ctx)
	if err != nil {
		logger.Error("Failed to list functions", "error", err)
		return err
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	semaphore := make(chan struct{}, 10)
	totalDeletedVersions := 0
	functionsProcessed := 0

	for _, function := range functions {
		if function.FunctionName == nil {
			continue
		}

		semaphore <- struct{}{}
		wg.Add(1)
		go func(functionName string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			deletedCount, err := j.cleanupFunction(ctx, functionName)
			if err != nil {
				logger.Error("Error cleaning function", "function", functionName, "error", err)
			} else {
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

	l.Start(handler)
}
