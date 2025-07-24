package syscap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// setupTestAPI creates a new SysCapAPI instance for testing
func setupTestAPI() *SysCapAPI {
	return &SysCapAPI{}
}

func registerTestSysCaps() {
	ClearCache()
	RegisterSysCap(NewSysCap(CategoryHardware, "gpu", "GPU", true, 1))
	RegisterSysCap(NewSysCap(CategorySoftware, "python", "Python", true, 1))
	RegisterSysCap(NewSysCap(CategoryOS, "macos", "macOS", true, 1))
}

func TestListAllSysCapsAPI(t *testing.T) {
	registerTestSysCaps()
	api := setupTestAPI()

	// Call the handler directly
	resp, err := api.ListAllSysCaps(context.Background(), &struct{}{})

	// Assert on the response
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.GreaterOrEqual(t, len(resp.Body), 3)
}

func TestListSysCapsByCategoryAPI(t *testing.T) {
	registerTestSysCaps()
	api := setupTestAPI()

	// Call the handler directly with valid category
	resp, err := api.ListSysCapsByCategory(context.Background(), &ListSysCapsByCategoryInput{
		Category: string(CategorySoftware),
	})

	// Assert on the response
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Len(t, resp.Body, 1)
	if len(resp.Body) > 0 {
		assert.Equal(t, "python", string(resp.Body[0].Name))
	}

	// Test with invalid category
	_, err = api.ListSysCapsByCategory(context.Background(), &ListSysCapsByCategoryInput{
		Category: "invalidcat",
	})

	// Assert that we get an error
	assert.Error(t, err)
	// The error is wrapped by Huma, so we can't directly type assert
	assert.Contains(t, err.Error(), "Invalid category")
}

func TestGetSysCapByTypeAndNameAPI(t *testing.T) {
	registerTestSysCaps()
	api := setupTestAPI()

	// Call the handler directly with valid category and name
	resp, err := api.GetSysCapByTypeAndName(context.Background(), &GetSysCapByTypeAndNameInput{
		Category: string(CategoryHardware),
		Name:     "gpu",
	})

	// Assert on the response
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "gpu", string(resp.Body.Name))

	// Test with non-existent capability
	_, err = api.GetSysCapByTypeAndName(context.Background(), &GetSysCapByTypeAndNameInput{
		Category: string(CategoryHardware),
		Name:     "doesnotexist",
	})

	// Assert that we get a 404 error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Test with invalid category
	_, err = api.GetSysCapByTypeAndName(context.Background(), &GetSysCapByTypeAndNameInput{
		Category: "invalidcat",
		Name:     "gpu",
	})

	// Assert that we get an error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid category")
}

func TestGetSysCapByKeyAPI(t *testing.T) {
	registerTestSysCaps()
	api := setupTestAPI()

	// Call the handler directly with valid key
	resp, err := api.GetSysCapByTypeAndName(context.Background(), &GetSysCapByTypeAndNameInput{
		Category: string(CategoryHardware),
		Name:     "gpu",
	})

	// Assert on the response
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "gpu", string(resp.Body.Name))

	// Test with invalid key format
	_, err = api.GetSysCapByTypeAndName(context.Background(), &GetSysCapByTypeAndNameInput{
		Category: "gpu", // Missing category
	})

	// Assert that we get a 400 error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid category")

	// Test with non-existent capability
	_, err = api.GetSysCapByTypeAndName(context.Background(), &GetSysCapByTypeAndNameInput{
		Category: string(CategoryHardware),
		Name:     "doesnotexist",
	})

	// Assert that we get a 404 error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRefreshAllSysCapsAPI(t *testing.T) {
	registerTestSysCaps()
	api := setupTestAPI()

	// Call the handler directly
	resp, err := api.RefreshAllSysCaps(context.Background(), &struct{}{})

	// Assert on the response
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.GreaterOrEqual(t, len(resp.Body), 3)
}

func TestRefreshSysCapsByCategoryAPI(t *testing.T) {
	registerTestSysCaps()
	api := setupTestAPI()

	// Call the handler directly with valid category
	resp, err := api.RefreshSysCapsByCategory(context.Background(), &ListSysCapsByCategoryInput{
		Category: string(CategoryHardware),
	})

	// Assert on the response
	assert.NoError(t, err)
	assert.NotNil(t, resp)

	found := false
	for _, c := range resp.Body {
		if c.Category == CategoryHardware {
			found = true
		}
	}
	assert.True(t, found)

	// Test with invalid category
	_, err = api.RefreshSysCapsByCategory(context.Background(), &ListSysCapsByCategoryInput{
		Category: "invalidcat",
	})

	// Assert that we get an error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid category")
}
