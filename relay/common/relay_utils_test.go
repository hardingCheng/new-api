package common

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeSeedanceDurationClampsOutOfRange(t *testing.T) {
	cases := []struct {
		req  TaskSubmitReq
		want int
	}{
		{req: TaskSubmitReq{Model: "seedance-2.0-480p", Seconds: "3"}, want: 4},
		{req: TaskSubmitReq{Model: "seedance-2.0-480p", Seconds: "16"}, want: 15},
		{req: TaskSubmitReq{Model: "doubao-seedance-1-0-lite-t2v", Duration: 3}, want: 4},
	}

	for _, tc := range cases {
		req := tc.req
		require.Nil(t, NormalizeSeedanceDuration(&req, &RelayInfo{}))
		require.Equal(t, tc.want, req.Duration)
		require.Equal(t, strconv.Itoa(tc.want), req.Seconds)
	}
}

func TestNormalizeSeedanceDurationKeepsBoundaryValues(t *testing.T) {
	cases := []TaskSubmitReq{
		{Model: "seedance-2.0-480p", Seconds: "4"},
		{Model: "seedance-2.0-480p", Seconds: "15"},
		{Model: "seedance-2.0-480p", Duration: 10},
		{Model: "seedance-2.0-480p"},
	}

	for _, req := range cases {
		require.NoError(t, func() error {
			if err := NormalizeSeedanceDuration(&req, &RelayInfo{}); err != nil {
				return err.Error
			}
			return nil
		}())
	}
}
