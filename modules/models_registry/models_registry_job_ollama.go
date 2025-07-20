package models_registry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hypernetix/hyperspot/libs/api_client"
	"github.com/hypernetix/hyperspot/modules/llm"
)

func ollamaModelsUpdate(ctx context.Context) (result *LLMModelRegistryJobResultEntry) {
	result = &LLMModelRegistryJobResultEntry{
		Updated: 0,
		Created: 0,
		Skipped: 0,
		Errors:  0,
		Total:   0,
		Error:   "",
	}

	api := api_client.NewBaseAPIClient("huggingface", "https://ollamadb.dev", 30, 30, true, false)
	resp, err := api.Get(ctx, "/api/v1/models?limit=1000", "")
	if err != nil {
		result.Error = fmt.Sprintf("failed to fetch models from %s: %v", api.GetBaseURL(), err)
		return result
	}

	var models struct {
		Models []map[string]interface{} `json:"models"`
	}
	if err := json.Unmarshal(resp.BodyBytes, &models); err != nil {
		result.Error = fmt.Sprintf("failed to parse models from %s: %v", api.GetBaseURL(), err)
		return result
	}

	// Update the database with models retrieved from Ollama.
	for _, m := range models.Models {
		model_identifier := strings.ToLower(fmt.Sprintf("%v", m["model_identifier"]))
		rm := LLMRegistryModel{
			LLMModel: llm.LLMModel{
				Name:      fmt.Sprintf("%v", m["model_name"]),
				Publisher: "ollama",
			},
			DBKey:         fmt.Sprintf("%s:%v", LLMRegistryOllama, model_identifier),
			Registry:      LLMRegistryOllama,
			ID:            model_identifier,
			ModelID:       model_identifier,
			Likes:         0,
			TrendingScore: 0,
			Downloads:     parseInt(m["pulls"]),
			CreatedAtMs:   time.Now().UTC().UnixMilli(),
			Tags:          "", // Parse if available.
			URL:           fmt.Sprintf("%v", m["model_url"]),
		}
		// If paging is required, you would loop through pages here.
		status, err := updateLLMRegistryModel(&rm)
		if err != nil {
			result.Error = fmt.Sprintf("failed to update model %s: %v", rm.DBKey, err)
			return result
		}

		if status == LLMModelRegistryUpdateStatusUpdated {
			result.Updated++
		} else if status == LLMModelRegistryUpdateStatusNew {
			result.Created++
		} else if status == LLMModelRegistryUpdateStatusSkipped {
			result.Skipped++
		} else if status == LLMModelRegistryUpdateStatusError {
			result.Errors++
		}
		result.Total++
	}
	return result
}
