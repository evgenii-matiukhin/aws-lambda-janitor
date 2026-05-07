package infra

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsevents"
	"github.com/aws/aws-cdk-go/awscdk/v2/awseventstargets"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdklambdagoalpha/v2"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type JanitorStackProps struct {
	awscdk.StackProps
	DryRun bool
}

type JanitorStack struct {
	awscdk.Stack
	JanitorFunction awslambda.IFunction
}

func NewJanitorStack(scope constructs.Construct, id string, props *JanitorStackProps) *JanitorStack {
	var sprops awscdk.StackProps
	if props != nil {
		sprops = props.StackProps
	}
	stack := awscdk.NewStack(scope, &id, &sprops)

	// Determine DRY_RUN environment variable
	dryRunValue := "false"
	if props != nil && props.DryRun {
		dryRunValue = "true"
	}

	// Create Lambda function using Go runtime
	janitorFunction := awscdklambdagoalpha.NewGoFunction(stack, jsii.String("JanitorFunction"), &awscdklambdagoalpha.GoFunctionProps{
		Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
		Entry:        jsii.String("cmd/janitor"),
		FunctionName: jsii.String("lambda-janitor"),
		Description:  jsii.String("Cleans up old Lambda function versions"),
		Timeout:      awscdk.Duration_Minutes(jsii.Number(15)),
		MemorySize:   jsii.Number(512),
		Environment: &map[string]*string{
			"DRY_RUN": jsii.String(dryRunValue),
		},
		Bundling: &awscdklambdagoalpha.BundlingOptions{
			GoBuildFlags: &[]*string{
				jsii.String("-ldflags=-s -w"),
			},
		},
	})

	// Grant Lambda permissions to manage other Lambda functions
	janitorFunction.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect: awsiam.Effect_ALLOW,
		Actions: jsii.Strings(
			"lambda:ListFunctions",
			"lambda:ListVersionsByFunction",
			"lambda:ListAliases",
			"lambda:DeleteFunction",
		),
		Resources: jsii.Strings("*"),
	}))

	// Create EventBridge rule to trigger Lambda on a weekly schedule
	// Runs every Sunday at 2 AM UTC
	rule := awsevents.NewRule(stack, jsii.String("JanitorScheduleRule"), &awsevents.RuleProps{
		RuleName:    jsii.String("lambda-janitor-weekly-schedule"),
		Description: jsii.String("Triggers Lambda Janitor weekly to clean up old versions"),
		Schedule: awsevents.Schedule_Cron(&awsevents.CronOptions{
			Minute:  jsii.String("0"),
			Hour:    jsii.String("2"),
			WeekDay: jsii.String("SUN"),
		}),
		Enabled: jsii.Bool(true),
	})

	// Add Lambda as target for the EventBridge rule
	rule.AddTarget(awseventstargets.NewLambdaFunction(janitorFunction, &awseventstargets.LambdaFunctionProps{}))

	// Output Lambda Function ARN
	awscdk.NewCfnOutput(stack, jsii.String("JanitorFunctionArn"), &awscdk.CfnOutputProps{
		Value:       janitorFunction.FunctionArn(),
		Description: jsii.String("ARN of the Lambda Janitor function"),
		ExportName:  jsii.String("LambdaJanitorFunctionArn"),
	})

	// Output Lambda Function Name
	awscdk.NewCfnOutput(stack, jsii.String("JanitorFunctionName"), &awscdk.CfnOutputProps{
		Value:       janitorFunction.FunctionName(),
		Description: jsii.String("Name of the Lambda Janitor function"),
		ExportName:  jsii.String("LambdaJanitorFunctionName"),
	})

	// Output Schedule Rule Name
	awscdk.NewCfnOutput(stack, jsii.String("ScheduleRuleName"), &awscdk.CfnOutputProps{
		Value:       rule.RuleName(),
		Description: jsii.String("Name of the EventBridge rule"),
		ExportName:  jsii.String("LambdaJanitorScheduleRule"),
	})

	return &JanitorStack{
		Stack:           stack,
		JanitorFunction: janitorFunction,
	}
}
