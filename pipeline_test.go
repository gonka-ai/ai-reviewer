package main

import (
	"encoding/json"
	"testing"
)

func TestExtractJSON(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "raw valid json",
			input: `{"applies":true,"why":"ok"}`,
			want:  `{"applies":true,"why":"ok"}`,
		},
		{
			name:  "json fenced block",
			input: "```json\n{\"applies\":true,\"why\":\"ok\"}\n```",
			want:  `{"applies":true,"why":"ok"}`,
		},
		{
			name:  "prefixed and suffixed chatter",
			input: "Sure, here it is:\n{\"applies\":true,\"why\":\"ok\"}\nThanks!",
			want:  `{"applies":true,"why":"ok"}`,
		},
		{
			name:    "malformed json remains invalid",
			input:   "Here:\n{\"applies\":true,\"why\":\"ok\"",
			want:    "{\"applies\":true,\"why\":\"ok\"",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractJSON(tc.input)
			if got != tc.want {
				t.Fatalf("extractJSON() = %q, want %q", got, tc.want)
			}

			var decoded any
			err := json.Unmarshal([]byte(got), &decoded)
			if tc.wantErr && err == nil {
				t.Fatalf("expected JSON unmarshal to fail for %q", got)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected valid JSON, got error %v", err)
			}
		})
	}
}
