package syscap

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/errorx"
)

// SysCapAPI provides API handlers for syscap operations
type SysCapAPI struct{}

// SysCapAPIResponse represents a single capability response
type SysCapAPIResponse struct {
	Body SysCap `json:"body"`
}

// SysCapListAPIResponse represents a list of syscaps response
type SysCapListAPIResponse struct {
	Body []SysCap `json:"body"`
}

// GetSysCapByKeyInput represents the input for getting a capability by key
type GetSysCapByKeyInput struct {
	Key string `path:"key" doc:"SysCap key in format 'category:name' (e.g., 'hardware:GPU', 'os:macOS')" example:"hardware:GPU"`
}

// GetSysCapByTypeAndNameInput represents the input for getting a capability by type and name
type GetSysCapByTypeAndNameInput struct {
	Category string `path:"category" doc:"SysCap category (hardware, software, os, llm_service, module)" example:"hardware"`
	Name     string `path:"name" doc:"SysCap name" example:"GPU"`
}

// ListSysCapsByCategoryInput represents the input for listing syscaps by category
type ListSysCapsByCategoryInput struct {
	Category string `path:"category" doc:"SysCap category (hardware, software, os, llm_service, module)" example:"hardware"`
}

// ListAllSysCaps handles the API request to list all system capabilities
func (api *SysCapAPI) ListAllSysCaps(ctx context.Context, input *struct{}) (*SysCapListAPIResponse, error) {
	syscaps, err := ListSysCaps("")
	if err != nil {
		return nil, errorx.NewErrInternalServerError("Failed to list syscaps: %v", err)
	}

	// Convert to slice of values instead of pointers
	result := make([]SysCap, len(syscaps))
	for i, cap := range syscaps {
		result[i] = *cap
	}

	return &SysCapListAPIResponse{
		Body: result,
	}, nil
}

// ListSysCapsByCategory handles the API request to list system capabilities by category
func (api *SysCapAPI) ListSysCapsByCategory(ctx context.Context, input *ListSysCapsByCategoryInput) (*SysCapListAPIResponse, error) {
	category := SysCapCategory(input.Category)

	// Validate category
	if err := isValidSysCapCategory(category); err != nil {
		return nil, err.HumaError()
	}

	syscaps, err := ListSysCaps(category)
	if err != nil {
		return nil, errorx.NewErrInternalServerError("Failed to list syscaps: %v", err)
	}

	// Convert to slice of values instead of pointers
	result := make([]SysCap, len(syscaps))
	for i, cap := range syscaps {
		result[i] = *cap
	}

	return &SysCapListAPIResponse{
		Body: result,
	}, nil
}

// GetSysCapByTypeAndName handles the API request to get a capability by category and name
func (api *SysCapAPI) GetSysCapByTypeAndName(ctx context.Context, input *GetSysCapByTypeAndNameInput) (*SysCapAPIResponse, error) {
	category := SysCapCategory(input.Category)
	name := SysCapName(input.Name)

	// Validate category
	if err := isValidSysCapCategory(category); err != nil {
		return nil, err.HumaError()
	}

	capability, err := GetSysCap(NewSysCapKey(category, name))
	if err != nil {
		if _, ok := err.(*SysCapNotFoundError); ok {
			return nil, errorx.NewErrNotFound("SysCap not found: %s:%s", input.Category, input.Name)
		}
		return nil, errorx.NewErrInternalServerError("Failed to get capability: %v", err)
	}

	return &SysCapAPIResponse{
		Body: *capability,
	}, nil
}

// RefreshAllSysCaps handles the API request to refresh all syscaps
func (api *SysCapAPI) RefreshAllSysCaps(ctx context.Context, input *struct{}) (*SysCapListAPIResponse, error) {
	// Clear the cache
	ClearCache()

	// Get fresh syscaps
	syscaps, err := ListSysCaps("")
	if err != nil {
		return nil, errorx.NewErrInternalServerError("Failed to refresh syscaps: %v", err)
	}

	// Convert to slice of values instead of pointers
	result := make([]SysCap, len(syscaps))
	for i, cap := range syscaps {
		result[i] = *cap
	}

	return &SysCapListAPIResponse{
		Body: result,
	}, nil
}

// RefreshSysCapsByCategory handles the API request to refresh syscaps for a category
func (api *SysCapAPI) RefreshSysCapsByCategory(ctx context.Context, input *ListSysCapsByCategoryInput) (*SysCapListAPIResponse, error) {
	category := SysCapCategory(input.Category)

	// Validate category
	if err := isValidSysCapCategory(category); err != nil {
		return nil, err.HumaError()
	}

	// Clear the cache for this category
	ClearCacheForCategory(category)

	// Get fresh syscaps for this category
	syscaps, err := ListSysCaps(category)
	if err != nil {
		return nil, errorx.NewErrInternalServerError("Failed to refresh syscaps: %v", err)
	}

	// Convert to slice of values instead of pointers
	result := make([]SysCap, len(syscaps))
	for i, cap := range syscaps {
		result[i] = *cap
	}

	return &SysCapListAPIResponse{
		Body: result,
	}, nil
}

// initSysCapsAPIRoutes initializes the syscaps API routes
func initSysCapsAPIRoutes(humaApi huma.API) {
	syscapAPI := &SysCapAPI{}

	// GET /syscaps - List all system syscaps
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "list-syscaps",
		Method:      http.MethodGet,
		Path:        "/syscaps",
		Summary:     "List all system capabilities",
		Tags:        []string{"SysCaps"},
	}, syscapAPI.ListAllSysCaps)

	// GET /syscaps/{category} - List syscaps by category
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "list-syscaps-by-category",
		Method:      http.MethodGet,
		Path:        "/syscaps/{category}",
		Summary:     "List system capabilities by category",
		Tags:        []string{"SysCaps"},
	}, syscapAPI.ListSysCapsByCategory)

	// GET /syscaps/{category}/{name} - Get capability by category and name
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "get-capability-by-type-and-name",
		Method:      http.MethodGet,
		Path:        "/syscaps/{category}/{name}",
		Summary:     "Get system capability by category and name",
		Tags:        []string{"SysCaps"},
	}, syscapAPI.GetSysCapByTypeAndName)

	// POST /syscaps/refresh - Clear cache and refresh all syscaps
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "refresh-syscaps",
		Method:      http.MethodPost,
		Path:        "/syscaps/refresh",
		Summary:     "Refresh all systemsyscaps (clear cache)",
		Tags:        []string{"SysCaps"},
	}, syscapAPI.RefreshAllSysCaps)

	// POST /syscaps/{category}/refresh - Clear cache and refresh syscaps for a category
	api.RegisterEndpoint(humaApi, huma.Operation{
		OperationID: "refresh-syscaps-by-category",
		Method:      http.MethodPost,
		Path:        "/syscaps/{category}/refresh",
		Summary:     "Refresh syscaps for a specific category (clear cache)",
		Tags:        []string{"SysCaps"},
	}, syscapAPI.RefreshSysCapsByCategory)
}
