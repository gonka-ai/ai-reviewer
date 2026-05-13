package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestGetModelClientKimiRequiresBaseURL(t *testing.T) {
	t.Chdir(t.TempDir())
	resetEnvCacheForTest()

	_, err := GetModelClient(context.Background(), ModelConfig{
		Provider:   "kimi",
		Model:      kimiK26GonkaModel,
		BaseURLEnv: "GONKA_BASE_URL",
	})
	if err == nil || !strings.Contains(err.Error(), "GONKA_BASE_URL not set") {
		t.Fatalf("expected missing GONKA_BASE_URL error, got %v", err)
	}
}

func TestGetModelClientKimiRequiresBaseURLEnv(t *testing.T) {
	t.Chdir(t.TempDir())
	resetEnvCacheForTest()

	_, err := GetModelClient(context.Background(), ModelConfig{
		Provider: "kimi",
		Model:    kimiK26GonkaModel,
	})
	if err == nil || !strings.Contains(err.Error(), "missing base_url_env") {
		t.Fatalf("expected missing base_url_env error, got %v", err)
	}
}

func TestGetModelClientKimiRejectsUnsupportedModel(t *testing.T) {
	t.Chdir(t.TempDir())
	resetEnvCacheForTest()
	t.Setenv("GONKA_BASE_URL", "http://example.test")

	_, err := GetModelClient(context.Background(), ModelConfig{
		Provider:   "kimi",
		Model:      "moonshotai/Kimi-K2.5",
		BaseURLEnv: "GONKA_BASE_URL",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported kimi model") {
		t.Fatalf("expected unsupported model error, got %v", err)
	}
}

func TestGetModelClientKimiRequiresAPIKeyWhenConfigured(t *testing.T) {
	t.Chdir(t.TempDir())
	resetEnvCacheForTest()
	t.Setenv("MOONSHOT_BASE_URL", "http://example.test")

	_, err := GetModelClient(context.Background(), ModelConfig{
		Provider:   "kimi",
		Model:      kimiK26MoonshotModel,
		BaseURLEnv: "MOONSHOT_BASE_URL",
		APIKeyEnv:  "MOONSHOT_API_KEY",
	})
	if err == nil || !strings.Contains(err.Error(), "MOONSHOT_API_KEY not set") {
		t.Fatalf("expected missing MOONSHOT_API_KEY error, got %v", err)
	}
}

func TestGetModelClientKimiAcceptsMoonshotModelName(t *testing.T) {
	t.Chdir(t.TempDir())
	resetEnvCacheForTest()
	t.Setenv("MOONSHOT_BASE_URL", "http://example.test")
	t.Setenv("MOONSHOT_API_KEY", "moonshot-token")

	client, err := GetModelClient(context.Background(), ModelConfig{
		Provider:   "kimi",
		Model:      kimiK26MoonshotModel,
		BaseURLEnv: "MOONSHOT_BASE_URL",
		APIKeyEnv:  "MOONSHOT_API_KEY",
	})
	if err != nil {
		t.Fatalf("expected Moonshot Kimi client construction to succeed, got %v", err)
	}
	if client == nil {
		t.Fatalf("expected Moonshot Kimi client, got nil")
	}
}

func TestGetModelClientKimiAcceptsOptionalGonkaAPIToken(t *testing.T) {
	t.Chdir(t.TempDir())
	resetEnvCacheForTest()
	t.Setenv("GONKA_BASE_URL", "http://example.test")
	t.Setenv("GONKA_API_TOKEN", "gonka-token")

	client, err := GetModelClient(context.Background(), ModelConfig{
		Provider:   "kimi",
		Model:      kimiK26GonkaModel,
		BaseURLEnv: "GONKA_BASE_URL",
	})
	if err != nil {
		t.Fatalf("expected Gonka Kimi client construction to succeed, got %v", err)
	}
	if client == nil {
		t.Fatalf("expected Gonka Kimi client, got nil")
	}
}

func TestGetModelClientExistingProvidersStillConstruct(t *testing.T) {
	t.Chdir(t.TempDir())
	resetEnvCacheForTest()
	t.Setenv("OPENAI_API_KEY", "test-openai")
	t.Setenv("ANTHROPIC_API_KEY", "test-anthropic")
	t.Setenv("GEMINI_API_KEY", "test-gemini")

	testCases := []ModelConfig{
		{Provider: "openai", Model: "gpt-4o"},
		{Provider: "anthropic", Model: "claude-3-5-sonnet-latest"},
		{Provider: "gemini", Model: "gemini-2.0-flash"},
	}

	for _, tc := range testCases {
		t.Run(tc.Provider, func(t *testing.T) {
			client, err := GetModelClient(context.Background(), tc)
			if err != nil {
				t.Fatalf("expected %s client construction to succeed, got %v", tc.Provider, err)
			}
			if client == nil {
				t.Fatalf("expected %s client, got nil", tc.Provider)
			}
		})
	}
}

func TestKimiClientGenerateWithUsageAndThinkingDisabled(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("expected no authorization header, got %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("failed decoding request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, "{\"id\":\"1\",\"object\":\"chat.completion\",\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"content\":\"Hello world\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":7,\"completion_tokens_details\":{\"reasoning_tokens\":0}}}")
	}))
	defer server.Close()

	client := NewKimiClient(server.URL, "", kimiK26GonkaModel, true, false)
	result, err := client.Generate(context.Background(), "hello", 123)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if result.Text != "Hello world" {
		t.Fatalf("expected text to be captured, got %q", result.Text)
	}
	if result.FinishReason != "stop" {
		t.Fatalf("expected finish reason stop, got %q", result.FinishReason)
	}
	if result.TokensIn != 11 || result.TokensOut != 7 {
		t.Fatalf("expected usage 11/7, got %d/%d", result.TokensIn, result.TokensOut)
	}
	if stream, ok := requestBody["stream"].(bool); !ok || stream {
		t.Fatalf("expected stream=false in request, got %#v", requestBody["stream"])
	}
	if maxTokens, ok := requestBody["max_completion_tokens"].(float64); !ok || int(maxTokens) != 123 {
		t.Fatalf("expected max_completion_tokens=123, got %#v", requestBody["max_completion_tokens"])
	}
	chatTemplateKwargs, ok := requestBody["chat_template_kwargs"].(map[string]any)
	if !ok || chatTemplateKwargs["thinking"] != false {
		t.Fatalf("expected chat_template_kwargs.thinking=false, got %#v", requestBody["chat_template_kwargs"])
	}
	if _, ok := requestBody["thinking"]; ok {
		t.Fatalf("did not expect top-level thinking for Gonka request, got %#v", requestBody["thinking"])
	}
}

func TestKimiClientGenerateWithoutUsageAndWithoutAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("expected no authorization header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, "{\"id\":\"1\",\"object\":\"chat.completion\",\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"content\":\"Hi there\",\"reasoning\":\"thinking\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":0,\"completion_tokens\":0}}")
	}))
	defer server.Close()

	client := NewKimiClient(server.URL, "", kimiK26GonkaModel, true, false)
	result, err := client.Generate(context.Background(), "hello", 0)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if result.Text != "Hi there" {
		t.Fatalf("expected text to be captured, got %q", result.Text)
	}
	if result.Reasoning != "thinking" {
		t.Fatalf("expected reasoning to be captured separately, got %q", result.Reasoning)
	}
	if result.TokensIn != 0 || result.TokensOut != 0 || result.TokensReasoning != 0 {
		t.Fatalf("expected zero usage when no usage details are present, got %+v", result)
	}
}

func TestKimiClientGenerateWithOptionalGonkaAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer gonka-token" {
			t.Fatalf("expected optional Gonka bearer auth header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, "{\"id\":\"1\",\"object\":\"chat.completion\",\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"content\":\"Gonka auth works\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1}}")
	}))
	defer server.Close()

	client := NewKimiClient(server.URL, "gonka-token", kimiK26GonkaModel, false, false)
	result, err := client.Generate(context.Background(), "hello", 0)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if result.Text != "Gonka auth works" {
		t.Fatalf("expected Gonka auth-protected text, got %q", result.Text)
	}
}

func TestKimiClientGenerateWithAuthAndThinkingEnabled(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer moonshot-token" {
			t.Fatalf("expected bearer auth header, got %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("failed decoding request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, "{\"id\":\"1\",\"object\":\"chat.completion\",\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"content\":\"Auth works\",\"reasoning_content\":\"deeper thinking\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":5,\"completion_tokens_details\":{\"reasoning_tokens\":2}}}")
	}))
	defer server.Close()

	client := NewKimiClient(server.URL, "moonshot-token", kimiK26MoonshotModel, false, true)
	result, err := client.Generate(context.Background(), "hello", 0)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if result.Text != "Auth works" {
		t.Fatalf("expected auth-protected text, got %q", result.Text)
	}
	if result.Reasoning != "deeper thinking" {
		t.Fatalf("expected reasoning_content capture, got %q", result.Reasoning)
	}
	if result.TokensReasoning != 2 {
		t.Fatalf("expected reasoning tokens to be captured, got %d", result.TokensReasoning)
	}
	if _, ok := requestBody["chat_template_kwargs"]; ok {
		t.Fatalf("did not expect chat_template_kwargs in Moonshot request, got %#v", requestBody["chat_template_kwargs"])
	}
	thinking, ok := requestBody["thinking"].(map[string]any)
	if !ok || thinking["type"] != "enabled" {
		t.Fatalf("expected Moonshot top-level thinking enabled, got %#v", requestBody["thinking"])
	}
}

func TestKimiClientGenerateWithReasoningContentField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, "{\"id\":\"1\",\"object\":\"chat.completion\",\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"content\":\"Done\",\"reasoning_content\":\"thinking\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1}}")
	}))
	defer server.Close()

	client := NewKimiClient(server.URL, "", kimiK26GonkaModel, true, false)
	result, err := client.Generate(context.Background(), "hello", 0)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if result.Text != "Done" {
		t.Fatalf("expected content to be preserved, got %q", result.Text)
	}
	if result.Reasoning != "thinking" {
		t.Fatalf("expected reasoning_content to be captured separately, got %q", result.Reasoning)
	}
}

func TestKimiClientGenerateJSONUsesNonStreamingPath(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("failed decoding request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, "{\"id\":\"1\",\"object\":\"chat.completion\",\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"content\":\"{\\\"ok\\\":true}\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":2}}")
	}))
	defer server.Close()

	client := NewKimiClient(server.URL, "", kimiK26GonkaModel, true, false)
	result, err := client.GenerateJSON(context.Background(), "Return JSON", 0)
	if err != nil {
		t.Fatalf("GenerateJSON returned error: %v", err)
	}

	if result.Text != "{\"ok\":true}" {
		t.Fatalf("expected JSON text, got %q", result.Text)
	}
	if stream, ok := requestBody["stream"].(bool); !ok || stream {
		t.Fatalf("expected stream=false in JSON request, got %#v", requestBody["stream"])
	}
	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("expected one message in request, got %#v", requestBody["messages"])
	}
	message, ok := messages[0].(map[string]any)
	if !ok {
		t.Fatalf("expected message object, got %#v", messages[0])
	}
	content, _ := message["content"].(string)
	if !strings.Contains(content, "Return exactly one valid JSON object or array") {
		t.Fatalf("expected JSON-only instruction to be appended, got %q", content)
	}
}

func resetEnvCacheForTest() {
	envCache = nil
	envOnce = sync.Once{}
}
