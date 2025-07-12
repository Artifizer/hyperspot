package llm

import (
	"context"
	"time"

	"github.com/hypernetix/hyperspot/libs/errorx"
	"github.com/hypernetix/hyperspot/libs/utils"
	"github.com/hypernetix/hyperspot/modules/job"
)

const (
	MODEL_JOB_TYPE_IMPORT  = "import"
	MODEL_JOB_TYPE_INSTALL = "install"
	MODEL_JOB_TYPE_LOAD    = "load"
	MODEL_JOB_TYPE_UNLOAD  = "unload"
	MODEL_JOB_TYPE_UPDATE  = "update"
	MODEL_JOB_TYPE_DELETE  = "delete"
)

var JOB_GROUP_LLM_MODEL_OPS = &job.JobGroup{
	Name:        "llm_model_ops",
	Description: "LLM management jobs",
	Queue:       job.JobQueueMaintenance,
}

// LLMModelJob represents a LLM model management job
type LLMModelJobParams struct {
	ModelName string `json:"model_name"`
}

// LLMModelJob represents a LLM model management job
type LLMServiceModelJobParams struct {
	ModelName   string `json:"model_name"`
	ServiceName string `json:"service_name"`
}

// LLMModelJobWorkerExecutor performs work for LLM model jobs.
func LLMModelJobWorkerExecutor(ctx context.Context, job *job.JobObj, progress chan<- float32) errorx.Error {
	jobParamsPtr := job.GetParamsPtr()
	if jobParamsPtr == nil {
		return errorx.NewErrInternalServerError("internal error, '%s' job does't have parameters set", job.GetType())
	}

	paramsPtr, ok := jobParamsPtr.(*LLMModelJobParams)
	if !ok {
		return errorx.NewErrInternalServerError("internal error, '%s' job parameters are not of type LLMModelJobParams", job.GetType())
	}

	var err error

	if job.GetTypePtr() == nil {
		return errorx.NewErrInternalServerError("internal error, '%s' job does't have type set", job.GetType())
	}

	switch job.GetTypePtr().Name {
	case MODEL_JOB_TYPE_LOAD:
		job.LogInfo("Loading model '%s'", paramsPtr.ModelName)
		_, err = LoadModel(ctx, paramsPtr.ModelName, progress)
	case MODEL_JOB_TYPE_UNLOAD:
		job.LogInfo("Unloading model '%s'", paramsPtr.ModelName)
		_, err = UnloadModel(ctx, paramsPtr.ModelName, progress)
	case MODEL_JOB_TYPE_UPDATE:
		job.LogInfo("Updating model '%s'", paramsPtr.ModelName)
		_, err = UpdateModel(ctx, paramsPtr.ModelName, progress)
	case MODEL_JOB_TYPE_DELETE:
		job.LogInfo("Deleting model '%s'", paramsPtr.ModelName)
		err = DeleteModel(ctx, paramsPtr.ModelName, progress)
	}

	if err != nil {
		return errorx.NewErrInternalServerError("internal error, failed to load model '%s': %s", paramsPtr.ModelName, err.Error())
	}

	if ctx.Err() != nil {
		return errorx.NewErrCanceled("job canceled")
	}

	return nil
}

// LLMModelJobWorker performs work for LLM model jobs.
func LLMServiceModelJobWorkerExecutor(ctx context.Context, job *job.JobObj, progress chan<- float32) errorx.Error {
	jobParamsPtr := job.GetParamsPtr()
	if jobParamsPtr == nil {
		return errorx.NewErrInternalServerError("internal error, '%s' job does't have parameters set", job.GetType())
	}

	paramsPtr, ok := jobParamsPtr.(*LLMServiceModelJobParams)
	if !ok {
		return errorx.NewErrInternalServerError("internal error, '%s' job parameters are not of type LLMServiceModelJobParams", job.GetType())
	}

	var model *LLMModel

	var serviceName string

	if paramsPtr.ServiceName == "" {
		serviceName = ServiceNameFromModelName(paramsPtr.ModelName)
	} else {
		serviceName = paramsPtr.ServiceName
	}

	service, err := GetServiceByName(serviceName)
	if err != nil {
		return errorx.NewErrBadRequest("service '%s' not found: %s", serviceName, err.Error())
	}

	switch job.GetTypePtr().Name {
	case MODEL_JOB_TYPE_IMPORT:
		job.LogInfo("Importing model '%s' into service '%s'", paramsPtr.ModelName, service.GetName())
		model, err = ImportModel(ctx, service.GetName(), paramsPtr.ModelName, progress)
	case MODEL_JOB_TYPE_INSTALL:
		job.LogInfo("Installing model '%s' into service '%s'", paramsPtr.ModelName, service.GetName())
		model, err = InstallModel(ctx, service.GetName(), paramsPtr.ModelName, progress)
	default:
		return errorx.NewErrInternalServerError("invalid job type: %s", job.GetType())
	}

	if err != nil {
		return errorx.NewErrInternalServerError("failed to import model '%s': %s", paramsPtr.ModelName, err.Error())
	}

	if ctx.Err() != nil {
		return errorx.NewErrCanceled("job canceled")
	}

	if model == nil {
		return errorx.NewErrNotFound("model '%s' not found", paramsPtr.ModelName)
	}

	return nil
}

func LLMModelJobWorkerParamsValidation(ctx context.Context, j *job.JobObj) errorx.Error {
	jobParamsPtr := j.GetParamsPtr()
	if jobParamsPtr == nil {
		return errorx.NewErrInternalServerError("internal error, '%s' job does't have parameters set", j.GetType())
	}

	paramsPtr, ok := jobParamsPtr.(*LLMModelJobParams)
	if !ok {
		paramsSchema, err := utils.StructToJSONString(paramsPtr)
		if err != nil {
			return errorx.NewErrInternalServerError("internal error, failed to convert job '%s' parameters to JSON string: %s", j.GetType(), err.Error())
		}
		expectedSchema, err := utils.StructToJSONString(&LLMModelJobParams{})
		if err != nil {
			return errorx.NewErrInternalServerError("internal error, failed to convert job '%s' expected schema to JSON string: %s", j.GetType(), err.Error())
		}
		return errorx.NewErrBadRequest("invalid job '%s' parameters '%s'; expected schema '%s'", j.GetType(), paramsSchema, expectedSchema)
	}

	model := GetModel(ctx, paramsPtr.ModelName, false, ModelStateAny)
	if model == nil {
		return errorx.NewErrBadRequest("model '%s' not found", paramsPtr.ModelName)
	}

	service := model.ServicePtr
	if service == nil {
		return errorx.NewErrBadRequest("service for model '%s' not found", paramsPtr.ModelName)
	}

	if j.GetTypePtr() == nil {
		return errorx.NewErrInternalServerError("internal error, '%s' job does't have type set", j.GetType())
	}

	capabilities := service.GetCapabilities()

	switch j.GetTypePtr().Name {
	case MODEL_JOB_TYPE_LOAD:
		if !capabilities.LoadModel {
			return errorx.NewErrNotImplemented("service '%s' does not support loading models", service.GetName())
		}
	case MODEL_JOB_TYPE_UNLOAD:
		if !capabilities.UnloadModel {
			return errorx.NewErrNotImplemented("service '%s' does not support unloading models", service.GetName())
		}
	case MODEL_JOB_TYPE_UPDATE:
		if !capabilities.UpdateModel {
			return errorx.NewErrNotImplemented("service '%s' does not support updating models", service.GetName())
		}
	case MODEL_JOB_TYPE_DELETE:
		if !capabilities.DeleteModel {
			return errorx.NewErrNotImplemented("service '%s' does not support deleting models", service.GetName())
		}
	default:
		return errorx.NewErrBadRequest("invalid job type: %s", j.GetType())
	}

	return nil
}

func LLMServiceModelJobParamsValidation(ctx context.Context, j *job.JobObj) errorx.Error {
	jobParamsPtr := j.GetParamsPtr()
	if jobParamsPtr == nil {
		return errorx.NewErrInternalServerError("internal error, '%s' job does't have parameters set", j.GetType())
	}

	paramsPtr, ok := jobParamsPtr.(*LLMServiceModelJobParams)
	if !ok {
		paramsSchema, err := utils.StructToJSONString(paramsPtr)
		if err != nil {
			return errorx.NewErrBadRequest("internal error, failed to convert job '%s' parameters to JSON string: %s", j.GetType(), err.Error())
		}
		expectedSchema, err := utils.StructToJSONString(&LLMModelJobParams{})
		if err != nil {
			return errorx.NewErrBadRequest("internal error, failed to convert job '%s' expected schema to JSON string: %s", j.GetType(), err.Error())
		}
		return errorx.NewErrBadRequest("invalid job '%s' parameters '%s'; expected schema '%s'", j.GetType(), paramsSchema, expectedSchema)
	}

	var serviceName string

	if paramsPtr.ServiceName == "" {
		serviceName = ServiceNameFromModelName(paramsPtr.ModelName)
	} else {
		serviceName = paramsPtr.ServiceName
	}

	service, err := GetServiceByName(serviceName)
	if err != nil {
		return errorx.NewErrBadRequest("service '%s' not found: %s", paramsPtr.ServiceName, err.Error())
	}

	if j.GetTypePtr() == nil {
		return errorx.NewErrInternalServerError("internal error, '%s' job does't have type set", j.GetType())
	}

	capabilities := service.GetCapabilities()

	switch j.GetTypePtr().Name {
	case MODEL_JOB_TYPE_IMPORT:
		if !capabilities.ImportModel {
			return errorx.NewErrNotImplemented("service '%s' does not support importing models", service.GetName())
		}
	case MODEL_JOB_TYPE_INSTALL:
		if !capabilities.InstallModel {
			return errorx.NewErrNotImplemented("service '%s' does not support installing models", service.GetName())
		}
	default:
		return errorx.NewErrInternalServerError("invalid job type: %s", j.GetType())
	}

	return nil
}

func initModelJobs() {
	// Register the LLM Model job worker.
	job.RegisterJobType(
		job.JobTypeParams{
			Group:                          JOB_GROUP_LLM_MODEL_OPS,
			Name:                           MODEL_JOB_TYPE_IMPORT,
			Description:                    "Import a model from a file",
			Params:                         &LLMServiceModelJobParams{},
			WorkerParamsValidationCallback: LLMServiceModelJobParamsValidation,
			WorkerExecutionCallback:        LLMServiceModelJobWorkerExecutor,
			WorkerStateUpdateCallback:      nil,
			Timeout:                        time.Hour * 3,
			MaxRetries:                     5,
			RetryDelay:                     time.Second * 5,
		},
	)

	job.RegisterJobType(
		job.JobTypeParams{
			Group:                          JOB_GROUP_LLM_MODEL_OPS,
			Name:                           MODEL_JOB_TYPE_INSTALL,
			Description:                    "Install a model from a remote site",
			Params:                         &LLMServiceModelJobParams{},
			WorkerParamsValidationCallback: LLMServiceModelJobParamsValidation,
			WorkerExecutionCallback:        LLMServiceModelJobWorkerExecutor,
			WorkerStateUpdateCallback:      nil,
			Timeout:                        time.Hour * 24,
			MaxRetries:                     5,
			RetryDelay:                     time.Second * 5,
		},
	)

	job.RegisterJobType(
		job.JobTypeParams{
			Group:                          JOB_GROUP_LLM_MODEL_OPS,
			Name:                           MODEL_JOB_TYPE_UPDATE,
			Description:                    "Update a model",
			Params:                         &LLMModelJobParams{},
			WorkerParamsValidationCallback: LLMModelJobWorkerParamsValidation,
			WorkerExecutionCallback:        LLMModelJobWorkerExecutor,
			WorkerStateUpdateCallback:      nil,
			Timeout:                        time.Hour * 8,
			MaxRetries:                     5,
			RetryDelay:                     time.Second * 5,
		},
	)

	job.RegisterJobType(
		job.JobTypeParams{
			Group:                          JOB_GROUP_LLM_MODEL_OPS,
			Name:                           MODEL_JOB_TYPE_LOAD,
			Description:                    "Load a model into memory",
			Params:                         &LLMModelJobParams{},
			WorkerParamsValidationCallback: LLMModelJobWorkerParamsValidation,
			WorkerExecutionCallback:        LLMModelJobWorkerExecutor,
			WorkerStateUpdateCallback:      nil,
			Timeout:                        time.Minute * 10,
			MaxRetries:                     5,
			RetryDelay:                     time.Second * 5,
		},
	)

	job.RegisterJobType(
		job.JobTypeParams{
			Group:                          JOB_GROUP_LLM_MODEL_OPS,
			Name:                           MODEL_JOB_TYPE_UNLOAD,
			Description:                    "Unload a model from memory",
			Params:                         &LLMModelJobParams{},
			WorkerParamsValidationCallback: LLMModelJobWorkerParamsValidation,
			WorkerExecutionCallback:        LLMModelJobWorkerExecutor,
			WorkerStateUpdateCallback:      nil,
			Timeout:                        time.Minute * 10,
			MaxRetries:                     5,
			RetryDelay:                     time.Second * 5,
		},
	)

	job.RegisterJobType(
		job.JobTypeParams{
			Group:                          JOB_GROUP_LLM_MODEL_OPS,
			Name:                           MODEL_JOB_TYPE_DELETE,
			Description:                    "Delete a model",
			Params:                         &LLMModelJobParams{},
			WorkerParamsValidationCallback: LLMModelJobWorkerParamsValidation,
			WorkerExecutionCallback:        LLMModelJobWorkerExecutor,
			WorkerStateUpdateCallback:      nil,
			Timeout:                        time.Minute * 10,
			MaxRetries:                     5,
			RetryDelay:                     time.Second * 5,
		})
}
