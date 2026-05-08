# AWS Lambda Janitor - CDK Project

A complete AWS CDK project in Go that deploys a Lambda function to automatically clean up old Lambda function versions, with GitHub Actions CI/CD using OIDC authentication.

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      GitHub Actions                          │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ 1. Authenticate via OIDC (no AWS keys needed)          │ │
│  │ 2. Deploy CDK stacks                                   │ │
│  └────────────────────────────────────────────────────────┘ │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
        ┌──────────────────────────────────────┐
        │      AWS Account                      │
        │  ┌────────────────────────────────┐  │
        │  │  OIDC Stack                    │  │
        │  │  • GitHub OIDC Provider        │  │
        │  │  • IAM Role for GitHub Actions │  │
        │  └────────────────────────────────┘  │
        │                                       │
        │  ┌────────────────────────────────┐  │
        │  │  Janitor Stack                 │  │
        │  │  • Lambda Function             │  │
        │  │  • EventBridge Rule (weekly)   │  │
        │  │  • IAM Permissions             │  │
        │  └────────────────────────────────┘  │
        └───────────────────────────────────────┘
```

## 📁 Project Structure

```
.
├── cmd/
│   └── janitor/
│       ├── main.go              # Lambda function code
│       └── main_test.go         # Unit tests
├── infra/
│   ├── oidc_stack.go           # OIDC Provider & IAM Role for GitHub
│   └── janitor_stack.go        # Lambda Janitor service
├── .github/
│   └── workflows/
│       └── deploy.yml          # GitHub Actions deployment workflow
├── build.sh                    # Local build & test script
├── deploy.sh                   # Local CDK deployment script
├── cdk.go                      # CDK app entry point
├── cdk.json                    # CDK configuration
├── go.mod                      # Go dependencies
├── go.sum                      # Go dependencies checksum
└── README_CDK.md              # This file
```

## 🚀 Features

### Lambda Janitor Function
- **Automatic Cleanup**: Scans all Lambda functions in the region
- **Smart Protection**: 
  - Never deletes `$LATEST` version
  - Never deletes versions with aliases (prod, staging, etc.)
  - Keeps the 3 most recent versions
- **Concurrent Processing**: Uses goroutines for parallel execution
- **Dry Run Mode**: Test without making actual deletions
- **Structured Logging**: JSON-formatted logs with detailed context
- **Pagination Support**: Handles large AWS accounts

### OIDC Authentication
- **Secure**: No AWS access keys stored in GitHub
- **GitHub OIDC Provider**: Configured with proper thumbprints
- **Scoped Trust**: Only allows your specific GitHub repository
- **Session Duration**: 1 hour maximum

### Automated Deployment
- **GitHub Actions**: Automated deployment on push to main
- **CDK Bootstrap**: Automatic setup of CDK toolkit
- **Testing**: Runs tests before deployment
- **Outputs**: Displays deployment information

## 📋 Prerequisites

1. **AWS Account** with appropriate permissions
2. **GitHub Repository** 
3. **Go 1.21+** installed
4. **Node.js 18+** and npm (for AWS CDK CLI)
5. **AWS CLI** configured locally (for initial setup)

## 🔧 Installation & Setup

### Step 1: Clone and Install Dependencies

```bash
# Clone the repository
git clone <your-repo-url>
cd aws-lambda-janitor

# Install Go dependencies
go mod download

# Install AWS CDK CLI globally
npm install -g aws-cdk
```

### Step 2: Configure Environment Variables

Update the following in your GitHub repository or locally:

```bash
export GITHUB_ORG="your-github-org"
export GITHUB_REPO="your-repo-name"
export AWS_ACCOUNT_ID="123456789012"
export AWS_REGION="us-east-1"
```

### Step 3: Initial Deployment (Manual)

For the first deployment, you need to deploy manually to create the OIDC role:

```bash
# Set AWS credentials (temporary, only needed for initial setup)
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"
export AWS_REGION="us-east-1"

# Bootstrap CDK (only needed once per account/region)
cdk bootstrap

# Deploy OIDC stack first
export GITHUB_ORG="no-org"
export GITHUB_REPO="aws-lambda-janitor"
cdk deploy LambdaJanitorOidcStack --require-approval never

# Get the Role ARN from outputs
aws cloudformation describe-stacks \
  --stack-name lambda-janitor-oidc \
  --query 'Stacks[0].Outputs[?OutputKey==`GitHubActionsRoleArn`].OutputValue' \
  --output text
```

### Step 4: Configure GitHub Secrets

Add these secrets to your GitHub repository (Settings → Secrets and variables → Actions):

- **`AWS_GITHUB_ACTIONS_ROLE_ARN`**: The Role ARN from step 3
- **`AWS_ACCOUNT_ID`**: Your AWS account ID

### Step 5: Deploy via GitHub Actions

```bash
# Push to main branch to trigger deployment
git add .
git commit -m "Initial CDK deployment"
git push origin main
```

## 🎯 Usage

### Manual Deployment

```bash
# Deploy all stacks
cdk deploy --all

# Deploy only OIDC stack
cdk deploy LambdaJanitorOidcStack

# Deploy only Janitor stack
cdk deploy LambdaJanitorStack

# Deploy with custom configuration
GITHUB_ORG="myorg" GITHUB_REPO="myrepo" DRY_RUN="true" cdk deploy --all
```

### Synthesize CloudFormation

```bash
# Generate CloudFormation templates
cdk synth

# View specific stack template
cdk synth LambdaJanitorStack
```

### Test Lambda Locally

```bash
# Run in dry-run mode
LOCAL_MODE=true DRY_RUN=true go run ./cmd/janitor

# Run with actual deletions (use with caution)
LOCAL_MODE=true go run ./cmd/janitor
```

### Invoke Deployed Lambda

```bash
# Invoke Lambda function manually
aws lambda invoke \
  --function-name lambda-janitor \
  --log-type Tail \
  --query 'LogResult' \
  --output text \
  response.json | base64 -d

# View CloudWatch Logs
aws logs tail /aws/lambda/lambda-janitor --follow
```

## 🔐 Security

### OIDC Trust Policy

The IAM role trusts only your specific GitHub repository:

```json
{
  "StringLike": {
    "token.actions.githubusercontent.com:sub": "repo:OWNER/REPO:*"
  },
  "StringEquals": {
    "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
  }
}
```

### Lambda Permissions

The Lambda function has minimal required permissions:
- `lambda:ListFunctions`
- `lambda:ListVersionsByFunction`
- `lambda:ListAliases`
- `lambda:DeleteFunction`

### GitHub Actions Permissions

The deployment role has permissions for:
- CloudFormation operations
- S3 (for CDK assets)
- IAM (for creating Lambda roles)
- Lambda operations
- EventBridge operations
- CloudWatch Logs

## 📊 Monitoring

### CloudWatch Logs

View Lambda execution logs:

```bash
aws logs tail /aws/lambda/lambda-janitor --follow
```

### CloudWatch Metrics

Monitor Lambda metrics:
- Invocations
- Errors
- Duration
- Throttles

### EventBridge Rule

The Lambda runs weekly on Sunday at 2 AM UTC. To change the schedule, modify `infra/janitor_stack.go`:

```go
Schedule: awsevents.Schedule_Cron(&awsevents.CronOptions{
    Minute:  jsii.String("0"),
    Hour:    jsii.String("2"),
    WeekDay: jsii.String("SUN"),
}),
```

## 🧪 Testing

```bash
# Run all tests
go test -v ./cmd/janitor/...

# Run tests with coverage
go test -cover ./cmd/janitor/...

# Run specific test
go test -run TestCleanupFunction -v ./cmd/janitor/...
```

## 🔄 Updating

### Update Lambda Code

1. Modify `cmd/janitor/main.go`
2. Commit and push to trigger deployment
3. Or deploy manually: `cdk deploy LambdaJanitorStack`

### Update Infrastructure

1. Modify files in `infra/` directory
2. Run `cdk diff` to preview changes
3. Deploy: `cdk deploy --all`

### Update CDK Dependencies

```bash
# Update Go modules
go get -u github.com/aws/aws-cdk-go/awscdk/v2@latest
go get -u github.com/aws/aws-cdk-go/awscdklambdagoalpha/v2@latest
go mod tidy
```

## 🗑️ Cleanup

To remove all resources:

```bash
# Destroy all stacks
cdk destroy --all

# Or destroy individually
cdk destroy LambdaJanitorStack
cdk destroy LambdaJanitorOidcStack
```

## 📝 Configuration Options

### Environment Variables

| Variable | Description | Default              | Required |
|----------|-------------|----------------------|----------|
| `GITHUB_ORG` | GitHub organization name | `no-org`             | Yes |
| `GITHUB_REPO` | GitHub repository name | `aws-lambda-janitor` | Yes |
| `CDK_DEFAULT_ACCOUNT` | AWS account ID | -                    | Yes |
| `CDK_DEFAULT_REGION` | AWS region | `us-east-1`          | No |
| `DRY_RUN` | Enable dry-run mode | `false`              | No |

### Lambda Configuration

Edit `infra/janitor_stack.go`:

```go
// Timeout (default: 15 minutes)
Timeout: awscdk.Duration_Minutes(jsii.Number(15)),

// Memory (default: 512 MB)
MemorySize: jsii.Number(512),

// Dry run mode
Environment: &map[string]*string{
    "DRY_RUN": jsii.String("false"),
},
```

### Retention Policy

Edit `cmd/janitor/main.go`:

```go
const (
    versionsToKeep = 3  // Change this value
)
```

## 🐛 Troubleshooting

### CDK Bootstrap Issues

```bash
# Re-bootstrap
cdk bootstrap --force
```

### OIDC Authentication Fails

1. Verify the Role ARN in GitHub secrets
2. Check trust policy matches your repository
3. Ensure GitHub token has correct permissions

### Lambda Execution Fails

1. Check CloudWatch Logs
2. Verify IAM permissions
3. Test locally with `LOCAL_MODE=true`

### Deployment Permission Errors

Ensure the GitHub Actions role has sufficient permissions (see `infra/oidc_stack.go`).

## 📚 Additional Resources

- [AWS CDK Go Documentation](https://docs.aws.amazon.com/cdk/v2/guide/work-with-cdk-go.html)
- [AWS Lambda Go Documentation](https://docs.aws.amazon.com/lambda/latest/dg/golang-handler.html)
- [GitHub Actions OIDC](https://docs.github.com/en/actions/deployment/security-hardening-your-deployments/configuring-openid-connect-in-amazon-web-services)

## 📄 License

See [LICENSE](LICENSE) file.

## 👥 Authors

Evgenii Matiukhin

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## 📞 Support

For issues and questions, please open an issue in the GitHub repository.

