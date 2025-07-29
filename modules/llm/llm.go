package llm

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/config"
	"github.com/hypernetix/hyperspot/libs/core"
	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/logging"
	"github.com/hypernetix/hyperspot/libs/utils"
	"github.com/hypernetix/hyperspot/modules/job"
)

var logger *logging.Logger = logging.MainLogger

var (
	llmMutex utils.DebugMutex
	// Map of URL to service instance
	urlToService = make(map[string]LLMService)
	// Map of service name to service instance
	nameToService = make(map[string]LLMService)
	allModels     = make(map[string]*LLMModel)
)

// CreateServiceInstance creates appropriate service instance based on configured type
func CreateServiceInstance(serviceType string, url string, serviceConfig config.ConfigLLMService, logger *logging.Logger) (LLMService, error) {
	// Check API key for external services
	switch serviceType {
	case "ollama":
		return NewOllamaService(url, serviceConfig, logger), nil
	case "lm_studio":
		return NewLMStudioService(url, serviceConfig, logger), nil
	case "jan":
		return NewJanService(url, serviceConfig, logger), nil
	case "localai":
		return NewLocalAIService(url, serviceConfig, logger), nil
	case "gpt4all":
		return NewGPT4AllService(url, serviceConfig, logger), nil
	case "openai":
		return NewOpenAIService(url, serviceConfig, logger), nil
	case "cortexso":
		return NewCortexSoService(url, serviceConfig, logger), nil
	case "anthropic":
		return NewAnthropicService(url, serviceConfig, logger), nil
	case "mock":
		return NewMockService(url, serviceConfig, logger), nil
	default:
		return nil, fmt.Errorf("unsupported service type: %s", serviceType)
	}
}

// generateServiceName creates a unique name for a service instance
func generateServiceName(baseType string, count int) string {
	if count == 1 {
		return baseType
	}
	return fmt.Sprintf("%s_%d", baseType, count)
}

// CreateServiceInstances initializes all configured LLM services
func CreateServiceInstances(ctx context.Context) error {
	var cfg *config.Config
	if cfg = config.Get(); cfg == nil {
		return fmt.Errorf("failed to get config")
	}

	// Track service type counts for naming
	serviceTypeCounts := make(map[string]int)

	// Create a wait group for parallel initialization
	var wg sync.WaitGroup
	errChan := make(chan error, len(cfg.LLM.LLMServices)+1)

	// Maps to collect registered services and models
	registeredURLMap := make(map[string]LLMService)
	registeredNameMap := make(map[string]LLMService)

	// Initialize each configured service
	for serviceType, serviceConfig := range cfg.LLM.LLMServices {
		for _, url := range serviceConfig.URLs {
			wg.Add(1)
			go func(url, serviceType string, serviceConfig config.ConfigLLMService) {
				defer wg.Done()

				// Create service instance
				service, err := CreateServiceInstance(serviceType, url, serviceConfig, logger)
				if err != nil {
					errChan <- fmt.Errorf("failed to create service instance for URL %s: %w", url, err)
					return
				}

				if service.IsExternal() {
					if serviceConfig.APIKey == "" {
						if serviceConfig.APIKeyEnvVar == "" {
							logging.Warn("API key environment variable is not configured for '%s' service, skipping",
								serviceType)
							return
						}
						if _, exists := os.LookupEnv(serviceConfig.APIKeyEnvVar); !exists {
							logging.Warn("API key environment variable %s not set for '%s' service, skipping",
								serviceConfig.APIKeyEnvVar, serviceType)
							return
						}
					} else {
						logging.Debug("Using provided API key for '%s' service", serviceType)
					}
				}

				logging.Debug("Checking availability of service %s at %s", service.GetName(), url)

				// Try to ping the service
				if err := service.Ping(ctx); err == nil {
					logging.Info("Detected service %s at %s, %s", service.GetName(), url, service.GetBaseURL())
				} else {
					if service.IsExternal() {
						logging.Error("Could not find service %s at %s: %v, check the API key env variable @ %s",
							service.GetName(), url, err, serviceConfig.APIKeyEnvVar)
					} else {
						logging.Debug("Could not find service %s at %s: %v", service.GetName(), url, err)
					}
				}

				llmMutex.Lock()
				defer llmMutex.Unlock()

				// Add to URL map
				registeredURLMap[url] = service

				// Generate unique name for the service
				serviceTypeCounts[serviceType]++
				serviceName := generateServiceName(serviceType, serviceTypeCounts[serviceType])
				registeredNameMap[serviceName] = service
			}(url, serviceType, serviceConfig)
		}
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Collect any errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	// Update the services registry
	llmMutex.Lock()
	urlToService = registeredURLMap
	nameToService = registeredNameMap
	llmMutex.Unlock()

	if len(errors) > 0 {
		return fmt.Errorf("service discovery completed with errors: %v", errors)
	}

	visitedService := ""

	for _, model := range ListAllModels(ctx, true) {
		if visitedService == "" {
			logging.Info("Discovered models:")
		}
		if model.ServicePtr.GetName() != visitedService {
			visitedService = model.ServicePtr.GetName()
			logging.Info("  Service: %s", visitedService)
		}
		var state string
		if model.State == ModelStateLoaded {
			state = " (LOADED)"
		}
		logging.Info("    - %s%s", model.Name, state)
	}

	return nil
}

// GetServiceByURL returns the LLM service with the given URL
func GetServiceByURL(url string) (LLMService, error) {
	llmMutex.Lock()
	defer llmMutex.Unlock()

	service, exists := urlToService[url]
	if !exists {
		return nil, fmt.Errorf("LLM service at %q not found", url)
	}
	return service, nil
}

// GetServiceByName returns the LLM service with the given name prefix
func GetServiceByName(namePrefix string) (LLMService, error) {
	llmMutex.Lock()
	defer llmMutex.Unlock()

	var firstMatch LLMService

	// Find all services that match the prefix
	for name, service := range nameToService {
		if strings.HasPrefix(name, namePrefix) {
			// Return immediately if we find a likely alive service
			if service.LikelyIsAlive() {
				return service, nil
			}
			// Keep track of the first match in case we don't find any alive services
			if firstMatch == nil {
				firstMatch = service
			}
		}
	}

	// If we found any match but none were alive, return the first match
	if firstMatch != nil {
		return firstMatch, nil
	}

	return nil, fmt.Errorf("LLM service with prefix %q not found", namePrefix)
}

func GetLLMService(ctx context.Context, name string) (LLMService, error) {
	if name == "" {
		return nil, fmt.Errorf("LLM service name is empty")
	}

	srv, err := GetServiceByName(name)
	if err == nil {
		return srv, nil
	}

	srv, err = GetServiceByURL(name)
	if err == nil {
		return srv, nil
	}

	return nil, fmt.Errorf("LLM service '%s' not found", name)
}

// ListServiceURLs returns a list of all registered service URLs
func ListServiceURLs() []string {
	llmMutex.Lock()
	defer llmMutex.Unlock()

	urls := make([]string, 0, len(urlToService))
	for url := range urlToService {
		urls = append(urls, url)
	}
	return urls
}

// ListServiceNames returns a list of all registered service names
func ListServiceNames() []string {
	llmMutex.Lock()
	defer llmMutex.Unlock()

	names := make([]string, 0, len(nameToService))
	for name := range nameToService {
		names = append(names, name)
	}
	return names
}

// ListAllServices returns a list of all registered service names
func ListAllServices(ctx context.Context) []string {
	return ListServiceNames()
}

// ListAliveServices returns a list of all registered alive service names
func ListAliveServices() []LLMService {
	llmMutex.Lock()
	defer llmMutex.Unlock()

	services := make([]LLMService, 0, len(nameToService))
	for _, service := range nameToService {
		if service.LikelyIsAlive() {
			services = append(services, service)
		}
	}
	return services
}

// ListAliveServices returns a list of all registered alive service names
func ListAliveServicesNames(ctx context.Context, pageRequest *api.PageAPIRequest) []string {
	llmMutex.Lock()
	defer llmMutex.Unlock()

	names := make([]string, 0, len(nameToService))
	for name, service := range nameToService {
		if service.LikelyIsAlive() {
			names = append(names, name)
		}
	}

	return api.PageAPIPaginate(names, pageRequest)
}

// UpdateModelsList updates the global list of available models
func UpdateModelsList(ctx context.Context) {
	// Check if context is already canceled or expired
	select {
	case <-ctx.Done():
		logging.Debug("Context %p canceled or expired before updating models list, returning: %v", ctx, ctx.Err())
		return
	default:
		// Context is still valid, continue execution
	}

	// Get all services
	services := ListAliveServices()

	if len(services) == 0 {
		logging.Warn("No alive LLM services found")
		allModels = make(map[string]*LLMModel)
		return
	}

	// Create a wait group for parallel processing
	var wg sync.WaitGroup

	// Channel to collect new models
	newModelsChan := make(chan *LLMModel)

	var m utils.DebugMutex

	// Process each service in parallel
	for _, service := range services {
		wg.Add(1)
		go func(service LLMService) {
			defer wg.Done()

			// Get models for the service
			serviceModels, err := service.ListModels(ctx, nil)
			if err != nil {
				logging.Warn("Failed to list models for service '%s': %v", service.GetName(), err)
				return
			}

			if len(serviceModels) == 0 {
				logging.Warn("No models found for service '%s'", service.GetName())
			}

			// Add models to the channel

			for _, model := range serviceModels {
				if model == nil {
					continue
				}
				m.Lock()
				newModelsChan <- model
				m.Unlock()
			}
		}(service)
	}

	// Close the channel when all workers are done
	go func() {
		wg.Wait()
		close(newModelsChan)
	}()

	llmMutex.Lock()
	defer llmMutex.Unlock()

	newModels := make([]*LLMModel, 0)
	for model := range newModelsChan {
		newModels = append(newModels, model)
	}

	for name := range allModels {
		found := false
		for _, model := range newModels {
			if model.Name == name {
				found = true
				break
			}
		}
		if !found {
			logging.Debug(fmt.Sprintf("Model '%s' hasdisappeared", name))
		}
	}

	for _, model := range newModels {
		found := false
		for name := range allModels {
			if name == model.Name {
				found = true
				break
			}
		}
		if !found {
			logging.Debug(fmt.Sprintf("New model '%s' has appeared", model.Name))
		}
	}

	allModels = make(map[string]*LLMModel)
	for _, model := range newModels {
		allModels[model.Name] = model
	}
}

// ListAllModels returns all available models
func ListAllModels(ctx context.Context, update bool) []*LLMModel {
	if update {
		UpdateModelsList(ctx)
	}

	llmMutex.Lock()
	defer llmMutex.Unlock()

	// Get all model names and sort them
	names := make([]string, 0, len(allModels))
	for name := range allModels {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]*LLMModel, 0, len(allModels))
	for _, name := range names {
		model := allModels[name]
		result = append(result, model)
	}
	return result
}

// ListLoadedModels returns all available models
func ListLoadedModels(ctx context.Context, update bool) []*LLMModel {
	models := ListAllModels(ctx, update)
	modelsOut := make([]*LLMModel, 0, len(models))

	for _, model := range models {
		if model.State == ModelStateLoaded {
			modelsOut = append(modelsOut, model)
		}
	}
	return modelsOut
}

// GetModel finds a model by name and given state, use ModelStateAny to ignore state
func GetModel(ctx context.Context, name string, exactMatch bool, state ModelState) *LLMModel {
	models := ListAllModels(ctx, false)

	// Try exact match first
	for _, model := range models {
		if model.Name == name {
			return model
		}
	}

	if exactMatch {
		return nil
	}

	// Try substring search
	for _, model := range models {
		if strings.Contains(model.Name, name) {
			if state == ModelStateAny || model.State == state {
				return model
			}
		}
	}

	return nil
}

func GetModelOrFirstLoaded(ctx context.Context, modelName string) (*LLMModel, errorx.Error) {
	var model *LLMModel

	if modelName == "" {
		models := ListLoadedModels(ctx, true)
		if len(models) == 0 {
			return nil, errorx.NewErrNotFound("No models found")
		}
		model = models[0]
		logging.Debug("'model' field is empty, using first loaded model: %s", model.UpstreamModelName)
	} else {
		model = GetModel(ctx, modelName, false, ModelStateLoaded)
		if model == nil {
			return nil, errorx.NewErrNotFound(fmt.Sprintf("Model '%s' not found", modelName))
		}
	}

	return model, nil
}

func importModel(ctx context.Context, j *job.JobObj, serviceName string, modelName string, progress chan<- float32) (*LLMModel, error) {
	service, err := GetServiceByName(serviceName)
	if err != nil {
		j.CancelRetry(ctx)
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	err = service.ImportModel(ctx, modelName, progress)
	if err != nil {
		return nil, fmt.Errorf("failed to import model: %w", err)
	}

	model, err := service.GetModel(ctx, modelName)
	if err != nil {
		return nil, fmt.Errorf("failed to get model: %w", err)
	}

	llmMutex.Lock()
	defer llmMutex.Unlock()
	allModels[model.Name] = model

	return model, nil
}

func installModel(ctx context.Context, j *job.JobObj, serviceName string, modelName string, progress chan<- float32) (*LLMModel, error) {
	service, err := GetServiceByName(serviceName)
	if err != nil {
		j.CancelRetry(ctx)
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	err = service.InstallModel(ctx, modelName, progress)
	if err != nil {
		return nil, fmt.Errorf("failed to install model: %w", err)
	}

	model, err := service.GetModel(ctx, modelName)
	if err != nil {
		return nil, fmt.Errorf("failed to get model: %w", err)
	}

	llmMutex.Lock()
	defer llmMutex.Unlock()
	allModels[model.Name] = model

	return model, nil
}

func updateModel(ctx context.Context, j *job.JobObj, modelName string, progress chan<- float32) (*LLMModel, error) {
	model := GetModel(ctx, modelName, false, ModelStateAny)
	if model == nil {
		return nil, fmt.Errorf("model '%s' not found", modelName)
	}

	service, err := GetServiceByName(model.Service)
	if err != nil {
		j.CancelRetry(ctx)
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	err = service.UpdateModel(ctx, model.UpstreamModelName, progress)
	if err != nil {
		return nil, fmt.Errorf("failed to update model: %w", err)
	}

	llmMutex.Lock()
	defer llmMutex.Unlock()
	allModels[model.Name] = model

	return model, nil
}

func loadModel(ctx context.Context, j *job.JobObj, modelName string, progress chan<- float32) (*LLMModel, error) {
	model := GetModel(ctx, modelName, true, ModelStateAny)
	if model == nil {
		j.CancelRetry(ctx)
		return nil, fmt.Errorf("model '%s' not found", modelName)
	}

	err := model.ServicePtr.LoadModel(ctx, model, progress)
	if err != nil {
		return nil, fmt.Errorf("failed to load model: %w", err)
	}

	return model, nil
}

func unloadModel(ctx context.Context, j *job.JobObj, modelName string, progress chan<- float32) (*LLMModel, error) {
	model := GetModel(ctx, modelName, true, ModelStateAny)
	if model == nil {
		j.CancelRetry(ctx)
		return nil, fmt.Errorf("model '%s' not found", modelName)
	}

	err := model.ServicePtr.UnloadModel(ctx, model, progress)
	if err != nil {
		return nil, fmt.Errorf("failed to unload model: %w", err)
	}

	return model, nil
}

func deleteModel(ctx context.Context, j *job.JobObj, modelName string, progress chan<- float32) errorx.Error {
	model := GetModel(ctx, modelName, false, ModelStateAny)
	if model == nil {
		j.CancelRetry(ctx)
		return errorx.NewErrNotFound(fmt.Sprintf("Model '%s' not found", modelName))
	}

	err := model.ServicePtr.DeleteModel(ctx, model.UpstreamModelName, progress)
	if err != nil {
		return errorx.NewErrInternalServerError(err.Error())
	}

	llmMutex.Lock()
	defer llmMutex.Unlock()
	delete(allModels, model.Name)

	return nil
}

func initLLMService() error {
	initModelJobs()
	return nil
}

func InitModule() {
	core.RegisterModule(&core.Module{
		InitMain:      initLLMService,
		InitAPIRoutes: registerLLMApiRoutes,
		Name:          "llm_services",
	})

	core.RegisterModule(&core.Module{
		InitAPIRoutes: registerOpenAIAPIRoutes,
		Name:          "openapi_api",
	})
}
