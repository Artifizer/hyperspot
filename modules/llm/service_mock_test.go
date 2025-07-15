package llm

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/openapi_client"
)

func TestRegisterMockServiceForUnitTests(t *testing.T) {
	ctx := context.Background()

	// Register the mock service
	RegisterMockServiceForUnitTests(ctx)

	// Verify that the service was registered
	services := ListServiceNames()
	assert.Contains(t, services, "mock", "Mock service should be registered")

	// Get the registered service
	service, err := GetServiceByName("mock")
	require.NoError(t, err, "Should be able to get the mock service")
	assert.Equal(t, "mock", service.GetName(), "Service name should be 'mock'")
	assert.False(t, service.IsExternal(), "Mock service should be internal")
}

func TestMockServiceListModels(t *testing.T) {
	ctx := context.Background()

	// Register the mock service if not already registered
	RegisterMockServiceForUnitTests(ctx)

	// Get the mock service
	service, err := GetServiceByName("mock")
	require.NoError(t, err, "Should be able to get the mock service")

	// List models
	pageRequest := &api.PageAPIRequest{
		PageSize:   10,
		PageNumber: 0,
	}
	models, err := service.ListModels(ctx, pageRequest)
	require.NoError(t, err, "ListModels should not return an error")
	assert.NotEmpty(t, models, "Should return some mock models")

	// Verify model properties
	if len(models) > 0 {
		model := models[0]
		assert.NotEmpty(t, model.Name, "Model should have a name")
		assert.True(t, model.Service == "mock", "Model should be from mock service")
	}
}

func TestMockServiceGetModel(t *testing.T) {
	ctx := context.Background()

	// Register the mock service if not already registered
	RegisterMockServiceForUnitTests(ctx)

	// Get the mock service
	service, err := GetServiceByName("mock")
	require.NoError(t, err, "Should be able to get the mock service")

	// Get a specific model
	model, err := service.GetModel(ctx, "mock-model-0.5B")
	require.NoError(t, err, "GetModel should not return an error")
	assert.Equal(t, "mock-model-0.5B", model.UpstreamModelName, "Should return the correct model")
	assert.Equal(t, "mock", model.Service, "Model should be from mock service")
}

func TestMockServiceLoadUnloadModel(t *testing.T) {
	ctx := context.Background()

	// Register the mock service if not already registered
	RegisterMockServiceForUnitTests(ctx)

	// Get the mock service
	service, err := GetServiceByName("mock")
	require.NoError(t, err, "Should be able to get the mock service")

	// Define model name
	modelName := "mock-model-0.5B"

	// Create progress channel for unload
	unloadProgress := make(chan float32)
	go func() {
		for progress := range unloadProgress {
			t.Logf("Unload progress: %.1f%%", progress)
		}
	}()

	// Unload the model first to ensure consistent state
	err = service.UnloadModel(ctx, &LLMModel{UpstreamModelName: modelName}, unloadProgress)
	require.NoError(t, err, "UnloadModel should not return an error")
	close(unloadProgress)

	// Get model to check its loaded status
	model, err := service.GetModel(ctx, modelName)
	require.NoError(t, err, "GetModel should not return an error")

	// If model's state is not "loaded", load it
	if model.State != "loaded" {
		// Create progress channel for load
		loadProgress := make(chan float32)
		go func() {
			for progress := range loadProgress {
				t.Logf("Load progress: %.1f%%", progress)
			}
		}()

		model := &LLMModel{
			Name: modelName,
		}

		err = service.LoadModel(ctx, model, loadProgress)
		require.NoError(t, err, "LoadModel should not return an error")
		close(loadProgress)

		// Verify model is loaded
		model, err = service.GetModel(ctx, modelName)
		require.NoError(t, err, "GetModel should not return an error")
		assert.Equal(t, "loaded", string(model.State), "Model should be loaded after LoadModel")
	}

	// Create another progress channel for final unload
	finalUnloadProgress := make(chan float32)
	go func() {
		for progress := range finalUnloadProgress {
			t.Logf("Final unload progress: %.1f%%", progress)
		}
	}()

	// Test unloading the model
	err = service.UnloadModel(ctx, model, finalUnloadProgress)
	require.NoError(t, err, "UnloadModel should not return an error")
	close(finalUnloadProgress)

	// Verify model is unloaded
	model, err = service.GetModel(ctx, modelName)
	require.NoError(t, err, "GetModel should not return an error")
	assert.NotEqual(t, "loaded", model.State, "Model should be unloaded after UnloadModel")
}

func TestMockServiceChatCompletions(t *testing.T) {
	ctx := context.Background()

	// Register the mock service if not already registered
	RegisterMockServiceForUnitTests(ctx)

	// Get the mock service
	service, err := GetServiceByName("mock")
	require.NoError(t, err, "Should be able to get the mock service")

	// Create a chat completion request
	req := &openapi_client.OpenAICompletionRequest{
		ModelName: "mock~mock-model-0.5B",
		Messages: []openapi_client.OpenAICompletionMessage{
			{
				Role:    "user",
				Content: "Hello, world!",
			},
		},
	}

	model := &LLMModel{
		UpstreamModelName: "mock~mock-model-0.5B",
	}

	// Get completions
	resp, err := service.ChatCompletions(ctx, model, req)
	require.NoError(t, err, "ChatCompletions should not return an error")
	assert.NotNil(t, resp, "Response should not be nil")
	assert.NotEmpty(t, resp.Choices, "Response should contain choices")

	if len(resp.Choices) > 0 {
		assert.NotEmpty(t, resp.Choices[0].Message.Content, "Response should have content")
	}
}

func TestMockServicePing(t *testing.T) {
	ctx := context.Background()

	// Register the mock service if not already registered
	RegisterMockServiceForUnitTests(ctx)

	// Get the mock service
	service, err := GetServiceByName("mock")
	require.NoError(t, err, "Should be able to get the mock service")

	// Ping the service
	err = service.Ping(ctx)
	require.NoError(t, err, "Ping should not return an error")
}

func TestMockServiceInstallModel(t *testing.T) {
	ctx := context.Background()

	// Register the mock service if not already registered
	RegisterMockServiceForUnitTests(ctx)

	// Get the mock service
	service, err := GetServiceByName("mock")
	require.NoError(t, err, "Should be able to get the mock service")

	modelName := "mock-test-model"

	// Create progress channel for install
	installProgress := make(chan float32)
	go func() {
		for progress := range installProgress {
			t.Logf("Install progress: %.1f%%", progress)
		}
	}()

	// Install the model
	err = service.InstallModel(ctx, modelName, installProgress)
	require.NoError(t, err, "InstallModel should not return an error")
	close(installProgress)

	// Verify the model is now available (in a real test, we might wait for installation)
	// This is a mock, so we assume it's immediately available
	model, err := service.GetModel(ctx, modelName)
	require.NoError(t, err, "GetModel should not return an error after installation")
	assert.Equal(t, "mock-test-model", model.UpstreamModelName, "Should have the correct model name")

	// Create progress channel for delete
	deleteProgress := make(chan float32)
	go func() {
		for progress := range deleteProgress {
			t.Logf("Delete progress: %.1f%%", progress)
		}
	}()

	err = service.DeleteModel(ctx, modelName, deleteProgress)
	require.NoError(t, err, "DeleteModel should not return an error")
	close(deleteProgress)
}

func TestMockServiceUpdateModel(t *testing.T) {
	ctx := context.Background()

	// Register the mock service if not already registered
	RegisterMockServiceForUnitTests(ctx)

	// Get the mock service
	service, err := GetServiceByName("mock")
	require.NoError(t, err, "Should be able to get the mock service")

	// Use a WaitGroup to ensure logging goroutines finish before the test exits
	var wg sync.WaitGroup

	// First ensure we have a model to update
	modelName := "mock-update-test-model"

	// Create progress channel for install
	installProgress := make(chan float32)
	wg.Add(1) // Increment WaitGroup counter
	go func() {
		defer wg.Done() // Decrement counter when goroutine finishes
		for progress := range installProgress {
			t.Logf("Install progress: %.1f%%", progress)
		}
	}()

	err = service.InstallModel(ctx, modelName, installProgress)
	require.NoError(t, err, "InstallModel should not return an error")
	close(installProgress)

	// Create progress channel for update
	updateProgress := make(chan float32)
	wg.Add(1) // Increment WaitGroup counter
	go func() {
		defer wg.Done() // Decrement counter when goroutine finishes
		for progress := range updateProgress {
			t.Logf("Update progress: %.1f%%", progress)
		}
	}()

	// Test updating the model
	err = service.UpdateModel(ctx, modelName, updateProgress)
	require.NoError(t, err, "UpdateModel should not return an error")
	close(updateProgress)

	// Verify the model is still available
	model, err := service.GetModel(ctx, modelName)
	require.NoError(t, err, "GetModel should not return an error after update")
	assert.Equal(t, modelName, model.UpstreamModelName, "Should have the correct model name")

	// Create progress channel for delete
	deleteProgress := make(chan float32)
	wg.Add(1) // Increment WaitGroup counter
	go func() {
		defer wg.Done() // Decrement counter when goroutine finishes
		for progress := range deleteProgress {
			t.Logf("Delete progress: %.1f%%", progress)
		}
	}()

	// Clean up
	err = service.DeleteModel(ctx, modelName, deleteProgress)
	require.NoError(t, err, "DeleteModel should not return an error")
	close(deleteProgress)

	wg.Wait() // Wait for all logging goroutines to finish
}

func TestMockServiceImportModel(t *testing.T) {
	ctx := context.Background()

	// Register the mock service if not already registered
	RegisterMockServiceForUnitTests(ctx)

	// Get the mock service
	service, err := GetServiceByName("mock")
	require.NoError(t, err, "Should be able to get the mock service")

	modelName := "mock-import-test-model"

	// Create progress channel for import
	importProgress := make(chan float32)
	go func() {
		for progress := range importProgress {
			t.Logf("Import progress: %.1f%%", progress)
		}
	}()

	// Test importing the model
	err = service.ImportModel(ctx, modelName, importProgress)
	require.NoError(t, err, "ImportModel should not return an error")
	close(importProgress)

	// Verify the model is now available
	model, err := service.GetModel(ctx, modelName)
	require.NoError(t, err, "GetModel should not return an error after import")
	assert.Equal(t, modelName, model.UpstreamModelName, "Should have the correct model name")

	// Create progress channel for delete
	deleteProgress := make(chan float32)
	go func() {
		for progress := range deleteProgress {
			t.Logf("Delete progress: %.1f%%", progress)
		}
	}()

	// Clean up
	err = service.DeleteModel(ctx, modelName, deleteProgress)
	require.NoError(t, err, "DeleteModel should not return an error")
	close(deleteProgress)
}

func TestMockServiceGetCapabilities(t *testing.T) {
	ctx := context.Background()

	// Register the mock service if not already registered
	RegisterMockServiceForUnitTests(ctx)

	// Get the mock service
	service, err := GetServiceByName("mock")
	require.NoError(t, err, "Should be able to get the mock service")

	// Get capabilities
	capabilities := service.GetCapabilities()
	require.NotNil(t, capabilities, "GetCapabilities should not return nil")

	// Verify capabilities based on the actual struct definition
	assert.True(t, capabilities.LoadModel, "Mock service should be able to load models")
	assert.True(t, capabilities.UnloadModel, "Mock service should be able to unload models")
	assert.True(t, capabilities.InstallModel, "Mock service should be able to install models")
	assert.True(t, capabilities.DeleteModel, "Mock service should be able to delete models")
	assert.True(t, capabilities.UpdateModel, "Mock service should be able to update models")
	assert.True(t, capabilities.ImportModel, "Mock service should be able to import models")
	assert.True(t, capabilities.HuggingfaceModelsSupported, "Mock service should support Huggingface models")
	assert.True(t, capabilities.OllamaModelsSupported, "Mock service should support Ollama models")
}

func TestMockServiceGetEmbeddings(t *testing.T) {
	ctx := context.Background()

	// Register the mock service if not already registered
	RegisterMockServiceForUnitTests(ctx)

	// Get the mock service
	service, err := GetServiceByName("mock")
	require.NoError(t, err, "Should be able to get the mock service")

	// Test case 1: Empty model should lead to an error
	t.Run("EmptyModel", func(t *testing.T) {
		// Create an empty model
		emptyModel := &LLMModel{}

		// Create embedding request
		req := &openapi_client.OpenAIEmbeddingRequest{
			Input: "This is a test input for embedding API",
		}

		// Call GetEmbeddings with empty model
		_, err := service.GetEmbeddings(ctx, emptyModel, req)
		assert.Error(t, err, "GetEmbeddings with empty model should return an error")
	})

	// Test case 2: Non-existing model should lead to an error
	t.Run("NonExistingModel", func(t *testing.T) {
		// Create a non-existing model
		nonExistingModel := &LLMModel{
			Name:              "non-existing-model",
			UpstreamModelName: "non-existing-model",
		}

		// Create embedding request
		req := &openapi_client.OpenAIEmbeddingRequest{
			Input: "This is a test input for embedding API",
		}

		// Call GetEmbeddings with non-existing model
		_, err := service.GetEmbeddings(ctx, nonExistingModel, req)
		assert.Error(t, err, "GetEmbeddings with non-existing model should return an error")
	})

	// Test case 3: LLM model should lead to an error (not an embedding model)
	t.Run("LLMModel", func(t *testing.T) {
		// Get a LLM model (not an embedding model)
		llmModel, err := service.GetModel(ctx, "mock-model-0.5B")
		require.NoError(t, err, "GetModel should not return an error")
		assert.Equal(t, ModelTypeLLM, llmModel.Type, "Model should be of LLM type")

		// Create embedding request
		req := &openapi_client.OpenAIEmbeddingRequest{
			Input: "This is a test input for embedding API",
		}

		// Call GetEmbeddings with LLM model
		_, err = service.GetEmbeddings(ctx, llmModel, req)
		assert.Error(t, err, "GetEmbeddings with LLM model should return an error")
		assert.Contains(t, err.Error(), "not an embedding model", "Error should mention model type issue")
	})

	// Test case 4: Embeddings must be returned if all parameters passed correctly
	t.Run("ValidEmbeddingModel", func(t *testing.T) {
		// Get an embedding model
		embeddingModel, err := service.GetModel(ctx, "mock-model-embeddings")
		require.NoError(t, err, "GetModel should not return an error")
		assert.Equal(t, ModelTypeEmbeddings, embeddingModel.Type, "Model should be of Embeddings type")

		// Create embedding request with string input
		req := &openapi_client.OpenAIEmbeddingRequest{
			Input: "This is a test input for embedding API",
		}

		// Call GetEmbeddings with valid embedding model
		resp, err := service.GetEmbeddings(ctx, embeddingModel, req)
		require.NoError(t, err, "GetEmbeddings with valid embedding model should not return an error")
		assert.NotNil(t, resp, "Response should not be nil")
		assert.NotEmpty(t, resp.Data, "Response should contain embedding data")
		assert.NotEmpty(t, resp.Data[0].Embedding, "Embedding vector should not be empty")

		// Test with dimensions parameter
		reqWithDimensions := &openapi_client.OpenAIEmbeddingRequest{
			Input:      "This is a test input for embedding API",
			Dimensions: 128,
		}

		// Call GetEmbeddings with dimensions specified
		respWithDimensions, err := service.GetEmbeddings(ctx, embeddingModel, reqWithDimensions)
		require.NoError(t, err, "GetEmbeddings with dimensions should not return an error")
		assert.NotNil(t, respWithDimensions, "Response should not be nil")
		assert.NotEmpty(t, respWithDimensions.Data, "Response should contain embedding data")
		assert.Equal(t, 128, len(respWithDimensions.Data[0].Embedding), "Embedding vector should have specified dimensions")

		// Test with array input
		arrayInput := []string{"First input", "Second input"}
		reqWithArrayInput := &openapi_client.OpenAIEmbeddingRequest{
			Input: arrayInput,
		}

		// Call GetEmbeddings with array input
		respWithArrayInput, err := service.GetEmbeddings(ctx, embeddingModel, reqWithArrayInput)
		require.NoError(t, err, "GetEmbeddings with array input should not return an error")
		assert.NotNil(t, respWithArrayInput, "Response should not be nil")
		assert.Equal(t, len(arrayInput), len(respWithArrayInput.Data), "Response should contain embedding data for each input")
		for i := 0; i < len(arrayInput); i++ {
			assert.Equal(t, i, respWithArrayInput.Data[i].Index, "Embedding index should match input index")
			assert.NotEmpty(t, respWithArrayInput.Data[i].Embedding, "Embedding vector should not be empty")
		}
	})
}
