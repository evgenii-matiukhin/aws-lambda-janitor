package main

import (
	"os"

	"aws-lambda-janitor/infra"
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/jsii-runtime-go"
)

func main() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)

	// Get configuration from environment variables or use defaults
	githubOrg := getEnv("GITHUB_ORG", "no-org")
	githubRepo := getEnv("GITHUB_REPO", "aws-lambda-janitor")
	awsAccount := getEnv("CDK_DEFAULT_ACCOUNT", "")
	awsRegion := getEnv("CDK_DEFAULT_REGION", "us-east-1")
	dryRun := getEnv("DRY_RUN", "false") == "true"

	env := &awscdk.Environment{
		Account: jsii.String(awsAccount),
		Region:  jsii.String(awsRegion),
	}

	// Create OIDC Stack for GitHub Actions authentication
	oidcStack := infra.NewOidcStack(app, "LambdaJanitorOidcStack", &infra.OidcStackProps{
		StackProps: awscdk.StackProps{
			Env:         env,
			Description: jsii.String("OIDC Provider and IAM Role for GitHub Actions"),
			StackName:   jsii.String("lambda-janitor-oidc"),
		},
		GitHubOrg:  githubOrg,
		GitHubRepo: githubRepo,
	})

	// Create Janitor Service Stack
	janitorStack := infra.NewJanitorStack(app, "LambdaJanitorStack", &infra.JanitorStackProps{
		StackProps: awscdk.StackProps{
			Env:         env,
			Description: jsii.String("Lambda function to clean up old Lambda versions"),
			StackName:   jsii.String("lambda-janitor"),
		},
		DryRun: dryRun,
	})

	// Add dependency to ensure OIDC stack is deployed first
	janitorStack.AddDependency(oidcStack.Stack, jsii.String("OIDC provider must be created first"))

	// Add tags to all resources
	awscdk.Tags_Of(app).Add(jsii.String("Project"), jsii.String("LambdaJanitor"), nil)
	awscdk.Tags_Of(app).Add(jsii.String("ManagedBy"), jsii.String("CDK"), nil)

	app.Synth(nil)
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
