package llamacpp

// CompletionRequest represents a text completion request payload for llama.cpp HTTP server.
type CompletionRequest struct {
	Prompt      string   `json:"prompt"`
	Grammar     string   `json:"grammar,omitempty"`
	Temperature float64  `json:"temperature,omitempty"`
	TopK        int      `json:"top_k,omitempty"`
	TopP        float64  `json:"top_p,omitempty"`
	NPredict    int      `json:"n_predict,omitempty"`
	Stream      bool     `json:"stream,omitempty"`
	Stop        []string `json:"stop,omitempty"`
}

// CompletionResponse represents a text completion response from llama.cpp HTTP server.
type CompletionResponse struct {
	Content     string `json:"content"`
	Model       string `json:"model"`
	Stop        bool   `json:"stop"`
	StoppedEOS  bool   `json:"stopped_eos"`
	StoppedLimit bool  `json:"stopped_limit"`
	StoppedWord bool   `json:"stopped_word"`
	StoppingWord string `json:"stopping_word"`
}

// ChatMessage represents a single message in chat completions.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionRequest represents OpenAI-compatible chat request payload for llama.cpp.
type ChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Grammar     string        `json:"grammar,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	TopK        int           `json:"top_k,omitempty"`
	TopP        float64       `json:"top_p,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// ChatCompletionResponse represents OpenAI-compatible chat response from llama.cpp.
type ChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int         `json:"index"`
		Message      ChatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
}
