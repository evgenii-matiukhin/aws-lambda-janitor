package infra

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type OidcStackProps struct {
	awscdk.StackProps
	GitHubOrg  string
	GitHubRepo string
}

type OidcStack struct {
	awscdk.Stack
	GitHubActionsRole awsiam.IRole
}

func NewOidcStack(scope constructs.Construct, id string, props *OidcStackProps) *OidcStack {
	var sprops awscdk.StackProps
	if props != nil {
		sprops = props.StackProps
	}
	stack := awscdk.NewStack(scope, &id, &sprops)

	// Create OIDC Provider for GitHub Actions
	oidcProvider := awsiam.NewOpenIdConnectProvider(stack, jsii.String("GitHubOidcProvider"), &awsiam.OpenIdConnectProviderProps{
		Url: jsii.String("https://token.actions.githubusercontent.com"),
		ClientIds: jsii.Strings(
			"sts.amazonaws.com",
		),
		Thumbprints: jsii.Strings(
			// GitHub Actions OIDC thumbprint
			"6938fd4d98bab03faadb97b34396831e3780aea1",
			"1c58a3a8518e8759bf075b76b750d4f2df264fcd",
		),
	})

	// Create IAM Role for GitHub Actions
	// Trust policy: Only allow the specific GitHub repository
	githubActionsRole := awsiam.NewRole(stack, jsii.String("GitHubActionsRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewFederatedPrincipal(
			oidcProvider.OpenIdConnectProviderArn(),
			&map[string]interface{}{
				"StringLike": map[string]string{
					"token.actions.githubusercontent.com:sub": "repo:" + props.GitHubOrg + "/" + props.GitHubRepo + ":*",
				},
				"StringEquals": map[string]string{
					"token.actions.githubusercontent.com:aud": "sts.amazonaws.com",
				},
			},
			jsii.String("sts:AssumeRoleWithWebIdentity"),
		),
		RoleName:           jsii.String("GitHubActionsDeploymentRole"),
		Description:        jsii.String("Role for GitHub Actions to deploy Lambda Janitor via CDK"),
		MaxSessionDuration: awscdk.Duration_Hours(jsii.Number(1)),
	})

	// Grant permissions for CDK deployment
	// Option 1: AdministratorAccess (use with caution in production)
	// githubActionsRole.AddManagedPolicy(
	// 	awsiam.ManagedPolicy_FromAwsManagedPolicyName(jsii.String("AdministratorAccess")),
	// )

	// Option 2: Scoped permissions for CDK deployment (recommended)
	githubActionsRole.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect: awsiam.Effect_ALLOW,
		Actions: jsii.Strings(
			// CloudFormation permissions
			"cloudformation:*",
			// S3 permissions for CDK assets
			"s3:*",
			// IAM permissions for creating roles
			"iam:*",
			// Lambda permissions
			"lambda:*",
			// EventBridge permissions
			"events:*",
			// CloudWatch Logs permissions
			"logs:*",
			// SSM for CDK context
			"ssm:GetParameter",
			"ssm:PutParameter",
			// STS for role assumption
			"sts:AssumeRole",
		),
		Resources: jsii.Strings("*"),
	}))

	// Output the Role ARN for use in GitHub Actions
	awscdk.NewCfnOutput(stack, jsii.String("GitHubActionsRoleArn"), &awscdk.CfnOutputProps{
		Value:       githubActionsRole.RoleArn(),
		Description: jsii.String("ARN of the IAM Role for GitHub Actions"),
		ExportName:  jsii.String("GitHubActionsRoleArn"),
	})

	return &OidcStack{
		Stack:             stack,
		GitHubActionsRole: githubActionsRole,
	}
}
