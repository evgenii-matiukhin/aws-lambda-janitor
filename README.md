# AWS Lambda Janitor

An AWS Lambda function written in Go that automatically cleans up old Lambda function versions to help manage costs and stay within AWS service quotas.

## Overview

The Lambda Janitor scans all Lambda functions in your AWS account and removes old, unused versions based on a configurable retention policy. It protects critical versions while cleaning up obsolete ones.

## Features

- 🔄 **Automatic Cleanup**: Scans all Lambda functions in the region
- 🛡️ **Smart Protection**: Never deletes `$LATEST` or aliased versions
- 📦 **Retention Policy**: Keeps the 5 most recent versions by default
- ⚡ **Concurrent Processing**: Uses goroutines for fast, parallel execution
- 🔍 **Dry Run Mode**: Test without making actual deletions
- 📊 **Structured Logging**: JSON-formatted logs with detailed context
- 📄 **Pagination Support**: Handles large AWS accounts with many functions

## Requirements

- Go 1.21 or higher
- AWS SDK for Go v2
- AWS credentials with appropriate permissions

## Installation

```bash
# Clone the repository
git clone <repository-url>
cd aws-lambda-janitor

# Install dependencies
go mod download

# Build
go build -o janitor main.go
```

## AWS Permissions

The Lambda function requires the following IAM permissions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "lambda:ListFunctions",
        "lambda:ListVersionsByFunction",
        "lambda:ListAliases",
        "lambda:DeleteFunction"
      ],
      "Resource": "*"
    }
  ]
}
```

## Configuration

### Environment Variables

- **`LOCAL_MODE`** (optional): Set to `true` to run the function locally without AWS Lambda runtime. Required for local testing.
  - Default: `false`
  - Example: `LOCAL_MODE=true`

- **`DRY_RUN`** (optional): Set to `true` to enable dry-run mode. The function will log what would be deleted without actually deleting anything.
  - Default: `false`
  - Example: `DRY_RUN=true`

### Retention Policy

The default retention policy keeps **5 most recent versions**. To change this, modify the `versionsToKeep` constant in `main.go`:

```go
const (
    versionsToKeep = 5  // Change this value
)
```

## Usage

### Local Testing

```bash
# Set AWS credentials
export AWS_REGION=us-east-1
export AWS_ACCESS_KEY_ID=your-access-key
export AWS_SECRET_ACCESS_KEY=your-secret-key

# Run in dry-run mode (recommended for first run)
LOCAL_MODE=true DRY_RUN=true go run main.go

# Run with actual deletions
LOCAL_MODE=true go run main.go

# Or use environment variables
export LOCAL_MODE=true
export DRY_RUN=true
go run main.go
```

### Deploy to AWS Lambda

```bash
# Build for Linux
GOOS=linux GOARCH=amd64 go build -o bootstrap main.go
zip function.zip bootstrap

# Deploy using AWS CLI
aws lambda create-function \
  --function-name lambda-janitor \
  --runtime provided.al2 \
  --handler bootstrap \
  --zip-file fileb://function.zip \
  --role arn:aws:iam::YOUR_ACCOUNT:role/lambda-janitor-role \
  --environment Variables={DRY_RUN=false} \
  --timeout 300 \
  --memory-size 512
```

Or use the provided deployment scripts:

```bash
# Build
./build.sh

# Deploy
./deploy.sh
```

### Schedule with EventBridge

Create an EventBridge rule to run the janitor on a schedule:

```bash
aws events put-rule \
  --name lambda-janitor-schedule \
  --schedule-expression "rate(1 day)"

aws events put-targets \
  --rule lambda-janitor-schedule \
  --targets "Id"="1","Arn"="arn:aws:lambda:REGION:ACCOUNT:function:lambda-janitor"

aws lambda add-permission \
  --function-name lambda-janitor \
  --statement-id EventBridgeInvoke \
  --action lambda:InvokeFunction \
  --principal events.amazonaws.com \
  --source-arn arn:aws:events:REGION:ACCOUNT:rule/lambda-janitor-schedule
```

## How It Works

1. **List Functions**: Retrieves all Lambda functions in the current AWS region
2. **Process Concurrently**: Uses goroutines with a semaphore (limit: 10 concurrent operations) to process functions in parallel
3. **For Each Function**:
   - Lists all versions
   - Lists all aliases
   - Identifies versions to protect:
     - `$LATEST` version
     - Any version associated with an alias (e.g., `prod`, `staging`)
     - The 5 most recent versions
   - Deletes remaining old versions
4. **Logging**: Outputs structured JSON logs with function names, versions, and status

## Log Output Examples

### Dry Run Mode

```json
{
  "time": "2026-01-08T10:00:00Z",
  "level": "INFO",
  "msg": "Running in DRY RUN mode - no deletions will be performed"
}
{
  "time": "2026-01-08T10:00:01Z",
  "level": "INFO",
  "msg": "DRY RUN: Would delete version",
  "function": "my-function",
  "version": "3",
  "status": "dry_run"
}
```

### Normal Operation

```json
{
  "time": "2026-01-08T10:00:00Z",
  "level": "INFO",
  "msg": "Processing function",
  "function": "my-function"
}
{
  "time": "2026-01-08T10:00:01Z",
  "level": "INFO",
  "msg": "Successfully deleted version",
  "function": "my-function",
  "version": "3",
  "status": "deleted"
}
{
  "time": "2026-01-08T10:00:02Z",
  "level": "INFO",
  "msg": "Cleanup completed for function",
  "function": "my-function",
  "deleted": 5,
  "skipped": 10
}
```

## Protection Rules

The janitor will **NEVER** delete:

1. ✅ The `$LATEST` version
2. ✅ Any version associated with an alias (e.g., `prod`, `staging`, `dev`)
3. ✅ The 5 most recent versions (configurable)

## Testing

Run the comprehensive test suite:

```bash
# Run all tests
go test -v

# Run with coverage
go test -cover -coverprofile=coverage.out

# View coverage report
go tool cover -html=coverage.out

# Run specific test
go test -run TestCleanupFunctionProtectsAliasedVersions -v

# Run with race detection
go test -race -v
```

### Test Coverage

The test suite includes:
- Unit tests for all core functions
- Mock AWS Lambda client
- Pagination handling tests
- Error handling tests
- Protection rule verification
- Concurrent processing tests

## Performance Considerations

- **Concurrency**: The janitor uses a semaphore to limit concurrent operations to 10 at a time, preventing API rate limiting
- **Pagination**: All AWS API calls handle pagination automatically, ensuring no data is missed in large accounts
- **Timeout**: Recommended Lambda timeout: 300 seconds (5 minutes) for large accounts

## Monitoring

Monitor the janitor's execution using:

- **CloudWatch Logs**: All structured logs are sent to CloudWatch
- **CloudWatch Metrics**: Track Lambda invocation metrics
- **CloudWatch Alarms**: Set up alarms for function errors or throttling

Example CloudWatch Insights query:

```
fields @timestamp, msg, function, version, status
| filter msg like /deleted/
| stats count() by function
```

## Troubleshooting

### Issue: Too many versions being deleted

**Solution**: Increase the `versionsToKeep` constant or ensure critical versions have aliases.

### Issue: Rate limiting errors

**Solution**: Reduce the semaphore limit in the `Run()` method (currently set to 10).

### Issue: Function timeout

**Solution**: Increase the Lambda timeout setting (recommended: 300 seconds for large accounts).

### Issue: Permissions errors

**Solution**: Verify the Lambda execution role has all required permissions listed in the AWS Permissions section.

## Best Practices

1. **Start with Dry Run**: Always test with `DRY_RUN=true` first
2. **Use Aliases**: Tag important versions with aliases (prod, staging, etc.)
3. **Schedule Regularly**: Run the janitor daily or weekly to prevent version buildup
4. **Monitor Logs**: Review CloudWatch logs regularly to ensure expected behavior
5. **Set Appropriate Retention**: Balance between cost savings and version history needs

## Cost Savings

Lambda versions consume storage space and count toward service quotas. Regular cleanup can:
- Reduce Lambda storage costs
- Prevent hitting the 75 GB code storage limit per region
- Improve function listing performance in the AWS Console

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

See [LICENSE](LICENSE) file for details.

## Authors

- Evgenii Matiukhin

## Changelog

### Version 1.0.0 (2026-01-08)
- Initial release
- Support for AWS SDK v2
- Concurrent processing with goroutines
- Dry run mode
- Structured JSON logging
- Comprehensive test suite
- Protection for $LATEST and aliased versions
- Configurable retention policy (default: 5 versions)

## Support

For issues, questions, or contributions, please open an issue in the repository.

