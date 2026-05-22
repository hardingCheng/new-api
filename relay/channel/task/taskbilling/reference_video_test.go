package taskbilling

import (
	"net/http"
	"testing"
)

func TestDurationSecondsFromHeadersCeil(t *testing.T) {
	tests := []struct {
		value string
		want  int
	}{
		{value: "12.3", want: 13},
		{value: "14.5", want: 15},
		{value: "15.2", want: 16},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			header := http.Header{}
			header.Set("X-Content-Duration", tt.value)
			if got := DurationSecondsFromHeaders(header); got != tt.want {
				t.Fatalf("DurationSecondsFromHeaders() = %d, want %d", got, tt.want)
			}
		})
	}
}
