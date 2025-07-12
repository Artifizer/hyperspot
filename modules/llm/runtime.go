package llm

// LLMRuntime represents an LLM runtimes like: llama.cpp, mlx, onnx, tensorflow, etc.
// Cortexso uses the 'engine' term
type LLMRuntime struct {
	Name string
}
