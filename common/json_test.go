package common

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJsonRawMessageToString(t *testing.T) {
	tests := []struct {
		name string
		data json.RawMessage
		want string
	}{
		{
			name: "object",
			data: json.RawMessage(`{"city":"Paris","days":0,"strict":false}`),
			want: `{"city":"Paris","days":0,"strict":false}`,
		},
		{
			name: "string",
			data: json.RawMessage(`"{\"city\":\"Paris\",\"days\":0,\"strict\":false}"`),
			want: `{"city":"Paris","days":0,"strict":false}`,
		},
		{
			name: "null",
			data: json.RawMessage(`null`),
			want: "",
		},
		{
			name: "empty",
			data: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, JsonRawMessageToString(tt.data))
		})
	}
}

func TestFriendlyJSONError(t *testing.T) {
	numberIntoUint := func() error {
		var v struct {
			MaxTokens uint `json:"max_tokens"`
		}
		return Unmarshal([]byte(`{"max_tokens":-1}`), &v)
	}()
	stringIntoSlice := func() error {
		var v struct {
			Messages []string `json:"messages"`
		}
		return Unmarshal([]byte(`{"messages":"oops"}`), &v)
	}()
	syntaxBroken := func() error {
		var v map[string]any
		return Unmarshal([]byte(`{"a":`), &v)
	}()
	require.Error(t, numberIntoUint)
	require.Error(t, stringIntoSlice)
	require.Error(t, syntaxBroken)

	tests := []struct {
		name        string
		err         error
		wantContain string
	}{
		{
			name:        "number into uint keeps field name",
			err:         numberIntoUint,
			wantContain: `invalid value for field "max_tokens" (got number -1)`,
		},
		{
			name:        "string into slice keeps field name",
			err:         stringIntoSlice,
			wantContain: `invalid value for field "messages" (got string)`,
		},
		{
			name:        "syntax error",
			err:         syntaxBroken,
			wantContain: "not valid JSON",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FriendlyJSONError(tt.err)
			require.Error(t, got)
			require.Contains(t, got.Error(), tt.wantContain)
			// API 边界不能泄露 Go 内部结构信息
			require.NotContains(t, got.Error(), "Go struct")
			require.NotContains(t, got.Error(), "cannot unmarshal")
		})
	}

	t.Run("nil passes through", func(t *testing.T) {
		require.NoError(t, FriendlyJSONError(nil))
	})
	t.Run("non-decode error passes through unchanged", func(t *testing.T) {
		plain := json.Unmarshal([]byte(`1`), nil)
		require.Equal(t, plain, FriendlyJSONError(plain))
	})
}
