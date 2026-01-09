package main

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockLambdaClient is a mock implementation of the Lambda client
type MockLambdaClient struct {
	mock.Mock
}

func (m *MockLambdaClient) ListFunctions(ctx context.Context, input *lambda.ListFunctionsInput, _ ...func(*lambda.Options)) (*lambda.ListFunctionsOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*lambda.ListFunctionsOutput), args.Error(1)
}

func (m *MockLambdaClient) ListVersionsByFunction(ctx context.Context, input *lambda.ListVersionsByFunctionInput, _ ...func(*lambda.Options)) (*lambda.ListVersionsByFunctionOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*lambda.ListVersionsByFunctionOutput), args.Error(1)
}

func (m *MockLambdaClient) ListAliases(ctx context.Context, input *lambda.ListAliasesInput, _ ...func(*lambda.Options)) (*lambda.ListAliasesOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*lambda.ListAliasesOutput), args.Error(1)
}

func (m *MockLambdaClient) DeleteFunction(ctx context.Context, input *lambda.DeleteFunctionInput, _ ...func(*lambda.Options)) (*lambda.DeleteFunctionOutput, error) {
	args := m.Called(ctx, input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*lambda.DeleteFunctionOutput), args.Error(1)
}

// Test listAllFunctions with pagination
func TestListAllFunctions(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)

	janitor := &LambdaJanitor{
		client: mockClient,
	}

	// First page
	mockClient.On("ListFunctions", ctx, mock.MatchedBy(func(input *lambda.ListFunctionsInput) bool {
		return input.Marker == nil
	})).Return(&lambda.ListFunctionsOutput{
		Functions: []lambdaTypes.FunctionConfiguration{
			{FunctionName: aws.String("function1")},
			{FunctionName: aws.String("function2")},
		},
		NextMarker: aws.String("marker1"),
	}, nil)

	// Second page
	mockClient.On("ListFunctions", ctx, mock.MatchedBy(func(input *lambda.ListFunctionsInput) bool {
		return input.Marker != nil && *input.Marker == "marker1"
	})).Return(&lambda.ListFunctionsOutput{
		Functions: []lambdaTypes.FunctionConfiguration{
			{FunctionName: aws.String("function3")},
		},
		NextMarker: nil,
	}, nil)

	functions, err := janitor.listAllFunctions(ctx)

	assert.NoError(t, err)
	assert.Len(t, functions, 3)
	assert.Equal(t, "function1", *functions[0].FunctionName)
	assert.Equal(t, "function2", *functions[1].FunctionName)
	assert.Equal(t, "function3", *functions[2].FunctionName)

	mockClient.AssertExpectations(t)
}

// Test listAllFunctions error handling
func TestListAllFunctionsError(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)

	janitor := &LambdaJanitor{
		client: mockClient,
	}

	mockClient.On("ListFunctions", ctx, mock.Anything).Return(nil, errors.New("API error"))

	functions, err := janitor.listAllFunctions(ctx)

	assert.Error(t, err)
	assert.Nil(t, functions)
	mockClient.AssertExpectations(t)
}

// Test listAllVersions with pagination
func TestListAllVersions(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)

	janitor := &LambdaJanitor{
		client: mockClient,
	}

	functionName := "test-function"

	// First page
	mockClient.On("ListVersionsByFunction", ctx, mock.MatchedBy(func(input *lambda.ListVersionsByFunctionInput) bool {
		return *input.FunctionName == functionName && input.Marker == nil
	})).Return(&lambda.ListVersionsByFunctionOutput{
		Versions: []lambdaTypes.FunctionConfiguration{
			{Version: aws.String("$LATEST")},
			{Version: aws.String("1")},
		},
		NextMarker: aws.String("marker1"),
	}, nil)

	// Second page
	mockClient.On("ListVersionsByFunction", ctx, mock.MatchedBy(func(input *lambda.ListVersionsByFunctionInput) bool {
		return *input.FunctionName == functionName && input.Marker != nil && *input.Marker == "marker1"
	})).Return(&lambda.ListVersionsByFunctionOutput{
		Versions: []lambdaTypes.FunctionConfiguration{
			{Version: aws.String("2")},
			{Version: aws.String("3")},
		},
		NextMarker: nil,
	}, nil)

	versions, err := janitor.listAllVersions(ctx, functionName)

	assert.NoError(t, err)
	assert.Len(t, versions, 4)
	assert.Equal(t, "$LATEST", *versions[0].Version)
	assert.Equal(t, "1", *versions[1].Version)
	assert.Equal(t, "2", *versions[2].Version)
	assert.Equal(t, "3", *versions[3].Version)

	mockClient.AssertExpectations(t)
}

// Test getAliasedVersions
func TestGetAliasedVersions(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)

	janitor := &LambdaJanitor{
		client: mockClient,
	}

	functionName := "test-function"

	mockClient.On("ListAliases", ctx, mock.MatchedBy(func(input *lambda.ListAliasesInput) bool {
		return *input.FunctionName == functionName
	})).Return(&lambda.ListAliasesOutput{
		Aliases: []lambdaTypes.AliasConfiguration{
			{Name: aws.String("prod"), FunctionVersion: aws.String("5")},
			{Name: aws.String("staging"), FunctionVersion: aws.String("4")},
		},
		NextMarker: nil,
	}, nil)

	aliasedVersions, err := janitor.getAliasedVersions(ctx, functionName)

	assert.NoError(t, err)
	assert.Len(t, aliasedVersions, 2)
	assert.True(t, aliasedVersions["5"])
	assert.True(t, aliasedVersions["4"])
	assert.False(t, aliasedVersions["3"])

	mockClient.AssertExpectations(t)
}

// Test cleanupFunction - keeps recent versions
func TestCleanupFunctionKeepsRecentVersions(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)

	janitor := &LambdaJanitor{
		client: mockClient,
	}

	functionName := "test-function"

	// Mock ListVersionsByFunction
	mockClient.On("ListVersionsByFunction", ctx, mock.Anything).Return(&lambda.ListVersionsByFunctionOutput{
		Versions: []lambdaTypes.FunctionConfiguration{
			{Version: aws.String("$LATEST")},
			{Version: aws.String("5")},
			{Version: aws.String("4")},
			{Version: aws.String("3")},
			{Version: aws.String("2")},
			{Version: aws.String("1")},
		},
		NextMarker: nil,
	}, nil)

	// Mock ListAliases - no aliases
	mockClient.On("ListAliases", ctx, mock.Anything).Return(&lambda.ListAliasesOutput{
		Aliases:    []lambdaTypes.AliasConfiguration{},
		NextMarker: nil,
	}, nil)

	// Only version 1 should be deleted (keeping 5 most recent: 5,4,3,2,1 - wait, that's 5 versions)
	// Actually with versionsToKeep=5, we keep versions 5,4,3,2,1, so nothing gets deleted in this case
	// Let's add more versions to test deletion

	_, err := janitor.cleanupFunction(ctx, functionName)

	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

// Test cleanupFunction - deletes old versions
func TestCleanupFunctionDeletesOldVersions(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)

	janitor := &LambdaJanitor{
		client: mockClient,
	}

	functionName := "test-function"

	// Mock ListVersionsByFunction with 10 versions
	mockClient.On("ListVersionsByFunction", ctx, mock.Anything).Return(&lambda.ListVersionsByFunctionOutput{
		Versions: []lambdaTypes.FunctionConfiguration{
			{Version: aws.String("$LATEST")},
			{Version: aws.String("10")},
			{Version: aws.String("9")},
			{Version: aws.String("8")},
			{Version: aws.String("7")},
			{Version: aws.String("6")},
			{Version: aws.String("5")},
			{Version: aws.String("4")},
			{Version: aws.String("3")},
			{Version: aws.String("2")},
			{Version: aws.String("1")},
		},
		NextMarker: nil,
	}, nil)

	// Mock ListAliases - no aliases
	mockClient.On("ListAliases", ctx, mock.Anything).Return(&lambda.ListAliasesOutput{
		Aliases:    []lambdaTypes.AliasConfiguration{},
		NextMarker: nil,
	}, nil)

	// Should delete versions 5,4,3,2,1 (keeping 10,9,8,7,6)
	mockClient.On("DeleteFunction", ctx, mock.MatchedBy(func(input *lambda.DeleteFunctionInput) bool {
		return *input.FunctionName == functionName && (*input.Qualifier == "5" || *input.Qualifier == "4" || *input.Qualifier == "3" || *input.Qualifier == "2" || *input.Qualifier == "1")
	})).Return(&lambda.DeleteFunctionOutput{}, nil)

	_, err := janitor.cleanupFunction(ctx, functionName)

	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

// Test cleanupFunction - protects aliased versions
func TestCleanupFunctionProtectsAliasedVersions(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)

	janitor := &LambdaJanitor{
		client: mockClient,
	}

	functionName := "test-function"

	// Mock ListVersionsByFunction
	mockClient.On("ListVersionsByFunction", ctx, mock.Anything).Return(&lambda.ListVersionsByFunctionOutput{
		Versions: []lambdaTypes.FunctionConfiguration{
			{Version: aws.String("$LATEST")},
			{Version: aws.String("10")},
			{Version: aws.String("9")},
			{Version: aws.String("8")},
			{Version: aws.String("7")},
			{Version: aws.String("6")},
			{Version: aws.String("5")},
			{Version: aws.String("4")},
			{Version: aws.String("3")},
			{Version: aws.String("2")},
			{Version: aws.String("1")},
		},
		NextMarker: nil,
	}, nil)

	// Mock ListAliases - version 3 is aliased as "prod"
	mockClient.On("ListAliases", ctx, mock.Anything).Return(&lambda.ListAliasesOutput{
		Aliases: []lambdaTypes.AliasConfiguration{
			{Name: aws.String("prod"), FunctionVersion: aws.String("3")},
		},
		NextMarker: nil,
	}, nil)

	// Should delete versions 5,4,2,1 but NOT 3 (aliased)
	mockClient.On("DeleteFunction", ctx, mock.MatchedBy(func(input *lambda.DeleteFunctionInput) bool {
		version := *input.Qualifier
		return *input.FunctionName == functionName && (version == "5" || version == "4" || version == "2" || version == "1")
	})).Return(&lambda.DeleteFunctionOutput{}, nil)

	_, err := janitor.cleanupFunction(ctx, functionName)

	assert.NoError(t, err)

	// Ensure version 3 was NOT deleted
	mockClient.AssertNotCalled(t, "DeleteFunction", ctx, mock.MatchedBy(func(input *lambda.DeleteFunctionInput) bool {
		return input.Qualifier != nil && *input.Qualifier == "3"
	}))

	mockClient.AssertExpectations(t)
}

// Test cleanupFunction - never deletes $LATEST
func TestCleanupFunctionNeverDeletesLatest(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)

	janitor := &LambdaJanitor{
		client: mockClient,
	}

	functionName := "test-function"

	// Mock ListVersionsByFunction
	mockClient.On("ListVersionsByFunction", ctx, mock.Anything).Return(&lambda.ListVersionsByFunctionOutput{
		Versions: []lambdaTypes.FunctionConfiguration{
			{Version: aws.String("$LATEST")},
			{Version: aws.String("1")},
		},
		NextMarker: nil,
	}, nil)

	// Mock ListAliases
	mockClient.On("ListAliases", ctx, mock.Anything).Return(&lambda.ListAliasesOutput{
		Aliases:    []lambdaTypes.AliasConfiguration{},
		NextMarker: nil,
	}, nil)

	_, err := janitor.cleanupFunction(ctx, functionName)

	assert.NoError(t, err)

	// Ensure $LATEST was never passed to DeleteFunction
	mockClient.AssertNotCalled(t, "DeleteFunction", ctx, mock.MatchedBy(func(input *lambda.DeleteFunctionInput) bool {
		return input.Qualifier != nil && *input.Qualifier == "$LATEST"
	}))

	mockClient.AssertExpectations(t)
}

// Test deleteVersion
func TestDeleteVersion(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)

	janitor := &LambdaJanitor{
		client: mockClient,
	}

	functionName := "test-function"
	version := "3"

	mockClient.On("DeleteFunction", ctx, &lambda.DeleteFunctionInput{
		FunctionName: aws.String(functionName),
		Qualifier:    aws.String(version),
	}).Return(&lambda.DeleteFunctionOutput{}, nil)

	err := janitor.deleteVersion(ctx, functionName, version)

	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

// Test deleteVersion error handling
func TestDeleteVersionError(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)

	janitor := &LambdaJanitor{
		client: mockClient,
	}

	functionName := "test-function"
	version := "3"

	mockClient.On("DeleteFunction", ctx, mock.Anything).Return(nil, errors.New("deletion failed"))

	err := janitor.deleteVersion(ctx, functionName, version)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "deletion failed")
	mockClient.AssertExpectations(t)
}

// Test Run method with multiple functions
func TestRun(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)

	janitor := &LambdaJanitor{
		client: mockClient,
	}

	// Mock ListFunctions
	mockClient.On("ListFunctions", ctx, mock.Anything).Return(&lambda.ListFunctionsOutput{
		Functions: []lambdaTypes.FunctionConfiguration{
			{FunctionName: aws.String("function1")},
			{FunctionName: aws.String("function2")},
		},
		NextMarker: nil,
	}, nil)

	// Mock for function1
	mockClient.On("ListVersionsByFunction", ctx, mock.MatchedBy(func(input *lambda.ListVersionsByFunctionInput) bool {
		return *input.FunctionName == "function1"
	})).Return(&lambda.ListVersionsByFunctionOutput{
		Versions: []lambdaTypes.FunctionConfiguration{
			{Version: aws.String("$LATEST")},
			{Version: aws.String("1")},
		},
		NextMarker: nil,
	}, nil)

	mockClient.On("ListAliases", ctx, mock.MatchedBy(func(input *lambda.ListAliasesInput) bool {
		return *input.FunctionName == "function1"
	})).Return(&lambda.ListAliasesOutput{
		Aliases:    []lambdaTypes.AliasConfiguration{},
		NextMarker: nil,
	}, nil)

	// Mock for function2
	mockClient.On("ListVersionsByFunction", ctx, mock.MatchedBy(func(input *lambda.ListVersionsByFunctionInput) bool {
		return *input.FunctionName == "function2"
	})).Return(&lambda.ListVersionsByFunctionOutput{
		Versions: []lambdaTypes.FunctionConfiguration{
			{Version: aws.String("$LATEST")},
			{Version: aws.String("2")},
		},
		NextMarker: nil,
	}, nil)

	mockClient.On("ListAliases", ctx, mock.MatchedBy(func(input *lambda.ListAliasesInput) bool {
		return *input.FunctionName == "function2"
	})).Return(&lambda.ListAliasesOutput{
		Aliases:    []lambdaTypes.AliasConfiguration{},
		NextMarker: nil,
	}, nil)

	err := janitor.Run(ctx)

	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}
