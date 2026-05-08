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

func newTestJanitor(mockClient *MockLambdaClient) *LambdaJanitor {
	return &LambdaJanitor{client: mockClient, dryRun: false}
}

func TestListAllFunctions(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)
	janitor := newTestJanitor(mockClient)

	mockClient.On("ListFunctions", ctx, mock.MatchedBy(func(input *lambda.ListFunctionsInput) bool {
		return input.Marker == nil
	})).Return(&lambda.ListFunctionsOutput{
		Functions: []lambdaTypes.FunctionConfiguration{
			{FunctionName: aws.String("function1")},
			{FunctionName: aws.String("function2")},
		},
		NextMarker: aws.String("marker1"),
	}, nil)

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

func TestListAllFunctionsError(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)
	janitor := newTestJanitor(mockClient)

	mockClient.On("ListFunctions", ctx, mock.Anything).Return(nil, errors.New("API error"))

	functions, err := janitor.listAllFunctions(ctx)

	assert.Error(t, err)
	assert.Nil(t, functions)
	mockClient.AssertExpectations(t)
}

func TestListAllVersions(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)
	janitor := newTestJanitor(mockClient)

	functionName := "test-function"

	mockClient.On("ListVersionsByFunction", ctx, mock.MatchedBy(func(input *lambda.ListVersionsByFunctionInput) bool {
		return *input.FunctionName == functionName && input.Marker == nil
	})).Return(&lambda.ListVersionsByFunctionOutput{
		Versions: []lambdaTypes.FunctionConfiguration{
			{Version: aws.String("$LATEST")},
			{Version: aws.String("1")},
		},
		NextMarker: aws.String("marker1"),
	}, nil)

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

func TestGetAliasedVersions(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)
	janitor := newTestJanitor(mockClient)

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

func TestCleanupFunctionKeepsRecentVersions(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)
	janitor := newTestJanitor(mockClient)

	functionName := "test-function"

	// Exactly versionsToKeep (3) numeric versions — nothing should be deleted
	mockClient.On("ListVersionsByFunction", ctx, mock.Anything).Return(&lambda.ListVersionsByFunctionOutput{
		Versions: []lambdaTypes.FunctionConfiguration{
			{Version: aws.String("$LATEST")},
			{Version: aws.String("3")},
			{Version: aws.String("2")},
			{Version: aws.String("1")},
		},
		NextMarker: nil,
	}, nil)

	mockClient.On("ListAliases", ctx, mock.Anything).Return(&lambda.ListAliasesOutput{
		Aliases:    []lambdaTypes.AliasConfiguration{},
		NextMarker: nil,
	}, nil)

	_, err := janitor.cleanupFunction(ctx, functionName)

	assert.NoError(t, err)
	mockClient.AssertNotCalled(t, "DeleteFunction")
	mockClient.AssertExpectations(t)
}

func TestCleanupFunctionDeletesOldVersions(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)
	janitor := newTestJanitor(mockClient)

	functionName := "test-function"

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

	mockClient.On("ListAliases", ctx, mock.Anything).Return(&lambda.ListAliasesOutput{
		Aliases:    []lambdaTypes.AliasConfiguration{},
		NextMarker: nil,
	}, nil)

	// Keep 10,9,8 (versionsToKeep=3); delete 7,6,5,4,3,2,1
	mockClient.On("DeleteFunction", ctx, mock.MatchedBy(func(input *lambda.DeleteFunctionInput) bool {
		q := *input.Qualifier
		return *input.FunctionName == functionName &&
			(q == "7" || q == "6" || q == "5" || q == "4" || q == "3" || q == "2" || q == "1")
	})).Return(&lambda.DeleteFunctionOutput{}, nil)

	_, err := janitor.cleanupFunction(ctx, functionName)

	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestCleanupFunctionProtectsAliasedVersions(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)
	janitor := newTestJanitor(mockClient)

	functionName := "test-function"

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

	mockClient.On("ListAliases", ctx, mock.Anything).Return(&lambda.ListAliasesOutput{
		Aliases: []lambdaTypes.AliasConfiguration{
			{Name: aws.String("prod"), FunctionVersion: aws.String("3")},
		},
		NextMarker: nil,
	}, nil)

	// Keep 10,9,8 (versionsToKeep=3) and 3 (aliased); delete 7,6,5,4,2,1
	mockClient.On("DeleteFunction", ctx, mock.MatchedBy(func(input *lambda.DeleteFunctionInput) bool {
		v := *input.Qualifier
		return *input.FunctionName == functionName &&
			(v == "7" || v == "6" || v == "5" || v == "4" || v == "2" || v == "1")
	})).Return(&lambda.DeleteFunctionOutput{}, nil)

	_, err := janitor.cleanupFunction(ctx, functionName)

	assert.NoError(t, err)

	mockClient.AssertNotCalled(t, "DeleteFunction", ctx, mock.MatchedBy(func(input *lambda.DeleteFunctionInput) bool {
		return input.Qualifier != nil && *input.Qualifier == "3"
	}))

	mockClient.AssertExpectations(t)
}

func TestCleanupFunctionNeverDeletesLatest(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)
	janitor := newTestJanitor(mockClient)

	functionName := "test-function"

	mockClient.On("ListVersionsByFunction", ctx, mock.Anything).Return(&lambda.ListVersionsByFunctionOutput{
		Versions: []lambdaTypes.FunctionConfiguration{
			{Version: aws.String("$LATEST")},
			{Version: aws.String("1")},
		},
		NextMarker: nil,
	}, nil)

	mockClient.On("ListAliases", ctx, mock.Anything).Return(&lambda.ListAliasesOutput{
		Aliases:    []lambdaTypes.AliasConfiguration{},
		NextMarker: nil,
	}, nil)

	_, err := janitor.cleanupFunction(ctx, functionName)

	assert.NoError(t, err)

	mockClient.AssertNotCalled(t, "DeleteFunction", ctx, mock.MatchedBy(func(input *lambda.DeleteFunctionInput) bool {
		return input.Qualifier != nil && *input.Qualifier == "$LATEST"
	}))

	mockClient.AssertExpectations(t)
}

func TestDeleteVersion(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)
	janitor := newTestJanitor(mockClient)

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

func TestDeleteVersionError(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)
	janitor := newTestJanitor(mockClient)

	functionName := "test-function"
	version := "3"

	mockClient.On("DeleteFunction", ctx, mock.Anything).Return(nil, errors.New("deletion failed"))

	err := janitor.deleteVersion(ctx, functionName, version)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "deletion failed")
	mockClient.AssertExpectations(t)
}

func TestRun(t *testing.T) {
	ctx := context.Background()
	mockClient := new(MockLambdaClient)
	janitor := newTestJanitor(mockClient)

	mockClient.On("ListFunctions", ctx, mock.Anything).Return(&lambda.ListFunctionsOutput{
		Functions: []lambdaTypes.FunctionConfiguration{
			{FunctionName: aws.String("function1")},
			{FunctionName: aws.String("function2")},
		},
		NextMarker: nil,
	}, nil)

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
