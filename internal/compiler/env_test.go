package compiler_test

import (
	"testing"

	"github.com/warpcondev/cuesix/internal/compiler"
)

func TestSubstituteAPISIX(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"API_HOST":  "env.example",
		"PORT_8080": "8080",
	}

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "uses env var",
			input: "host: ${{ API_HOST }}\n",
			want:  "host: env.example\n",
		},
		{
			name:  "uses default when missing",
			input: "missing: ${{ MISSING := /default }}\n",
			want:  "missing: /default\n",
		},
		{
			name:  "default supports semicolon",
			input: "missing: ${{ MISSING := 127.0.0.1:8080 }}\n",
			want:  "missing: 127.0.0.1:8080\n",
		},
		{
			name:  "missing without default becomes empty",
			input: "empty: ${{ MISSING }}\n",
			want:  "empty: \n",
		},
		{
			name:  "blank name uses default",
			input: "blank: ${{ := /blank }}\n",
			want:  "blank: /blank\n",
		},
		{
			name:  "whitespace trimmed",
			input: "trim: ${{  API_HOST   }}\n",
			want:  "trim: env.example\n",
		},
		{
			name:  "default ignored when env present",
			input: "host: ${{ API_HOST := /default }}\n",
			want:  "host: env.example\n",
		},
		{
			name:  "name with digits and underscores",
			input: "port: ${{ PORT_8080 }}\n",
			want:  "port: 8080\n",
		},
		{
			name:  "non-matching text unchanged",
			input: "no change here\n",
			want:  "no change here\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := compiler.SubstituteAPISIX(tc.input, env); got != tc.want {
				t.Fatalf("unexpected substitution: %q", got)
			}
		})
	}
}
